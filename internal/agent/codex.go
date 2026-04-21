package agent

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yjwong/lark-cli/internal/api"
	"github.com/yjwong/lark-cli/internal/inbound"
)

type Config struct {
	Enabled        bool
	CodexBinary    string
	Workspace      string
	Model          string
	AckText        string
	ResultMaxChars int
	Timeout        time.Duration
}

type Runner struct {
	cfg    Config
	client *api.Client
	logger *log.Logger
}

func NewRunner(cfg Config, logger *log.Logger) *Runner {
	if logger == nil {
		logger = log.New(os.Stderr, "lark-agent: ", log.LstdFlags)
	}
	if cfg.CodexBinary == "" {
		cfg.CodexBinary = "codex"
	}
	if cfg.AckText == "" {
		cfg.AckText = "收到，开始处理。"
	}
	if cfg.ResultMaxChars <= 0 {
		cfg.ResultMaxChars = 1800
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Minute
	}

	return &Runner{
		cfg:    cfg,
		client: api.NewClient(),
		logger: logger,
	}
}

func (r *Runner) Enabled() bool {
	return r != nil && r.cfg.Enabled
}

func (r *Runner) Dispatch(entry inbound.LoggedEvent) {
	if !r.Enabled() {
		return
	}
	if !inbound.ShouldAutoReply(entry) {
		return
	}
	if strings.TrimSpace(entry.MessageText) == "" {
		return
	}

	go r.run(entry)
}

func (r *Runner) run(entry inbound.LoggedEvent) {
	if err := r.reply(entry, r.cfg.AckText); err != nil {
		r.logger.Printf("failed to send ack for message_id=%s: %v", entry.MessageID, err)
	}

	result, err := r.execute(entry)
	if err != nil {
		r.logger.Printf("codex task failed for message_id=%s: %v", entry.MessageID, err)
		result = "处理失败：" + trimForFeishu(err.Error(), r.cfg.ResultMaxChars)
	}

	if err := r.reply(entry, result); err != nil {
		r.logger.Printf("failed to send final reply for message_id=%s: %v", entry.MessageID, err)
	}
}

func (r *Runner) execute(entry inbound.LoggedEvent) (string, error) {
	tempDir, err := os.MkdirTemp("", "lark-codex-agent-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	outputFile := filepath.Join(tempDir, "last-message.txt")
	prompt := buildPrompt(entry, r.cfg.ResultMaxChars)

	args := []string{
		"-a", "never",
		"-s", "workspace-write",
		"exec",
		"-C", r.cfg.Workspace,
		"--skip-git-repo-check",
		"--output-last-message", outputFile,
		prompt,
	}
	if strings.TrimSpace(r.cfg.Model) != "" {
		args = append([]string{"-m", strings.TrimSpace(r.cfg.Model)}, args...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.cfg.CodexBinary, args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("任务超时，超过 %s", r.cfg.Timeout)
	}
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", trimForFeishu(msg, r.cfg.ResultMaxChars))
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		return "", fmt.Errorf("读取 Codex 输出失败: %w", err)
	}

	result := strings.TrimSpace(string(data))
	if result == "" {
		return "", fmt.Errorf("Codex 没有返回结果")
	}

	return trimForFeishu(result, r.cfg.ResultMaxChars), nil
}

func (r *Runner) reply(entry inbound.LoggedEvent, text string) error {
	content, err := buildTextContent(trimForFeishu(text, r.cfg.ResultMaxChars))
	if err != nil {
		return err
	}
	_, err = r.client.ReplyMessage(entry.MessageID, "text", content, entry.RootID, true)
	return err
}

func buildPrompt(entry inbound.LoggedEvent, resultMaxChars int) string {
	return strings.TrimSpace(fmt.Sprintf(`
你是一个本地 Codex 执行代理，这次任务来自飞书聊天消息。

要求：
- 直接处理用户请求，尽量少讲方案、多做事。
- 默认使用中文回复，输出要适合直接发回飞书。
- 如果任务能执行，就执行后汇报结果。
- 如果缺少关键信息或存在明显风险，只说最关键的阻塞点。
- 最终回复尽量控制在 %d 个字符以内。

上下文：
- chat_id: %s
- sender_open_id: %s
- message_id: %s

用户消息：
%s
`, resultMaxChars, entry.ChatID, entry.SenderOpenID, entry.MessageID, entry.MessageText))
}

func trimForFeishu(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:max])) + "\n\n[已截断]"
}

func buildTextContent(text string) (string, error) {
	return fmt.Sprintf(`{"text":%q}`, text), nil
}
