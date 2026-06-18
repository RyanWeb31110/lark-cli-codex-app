package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	prompt := r.buildPrompt(entry, r.cfg.ResultMaxChars)

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

	codexBinary, err := resolveCodexBinary(r.cfg.CodexBinary)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, codexBinary, args...)
	cmd.Env = codexEnv(os.Environ())
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

func resolveCodexBinary(configured string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(configured) != "" {
		candidates = append(candidates, strings.TrimSpace(configured))
	}
	candidates = append(candidates,
		"/opt/homebrew/bin/codex",
		"/Users/macmini_no1/bin/codex",
		"codex",
	)

	for _, candidate := range candidates {
		if filepath.IsAbs(candidate) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
				return candidate, nil
			}
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("找不到 Codex CLI，请确认 /opt/homebrew/bin/codex 或 ~/bin/codex 存在")
}

func codexEnv(base []string) []string {
	const prefix = "/opt/homebrew/bin:/opt/homebrew/sbin:/Users/macmini_no1/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	env := make([]string, 0, len(base)+1)
	replaced := false
	for _, item := range base {
		if strings.HasPrefix(item, "PATH=") {
			env = append(env, "PATH="+prefix+":"+strings.TrimPrefix(item, "PATH="))
			replaced = true
			continue
		}
		env = append(env, item)
	}
	if !replaced {
		env = append(env, "PATH="+prefix)
	}
	return env
}

func (r *Runner) reply(entry inbound.LoggedEvent, text string) error {
	content, err := buildTextContent(trimForFeishu(text, r.cfg.ResultMaxChars))
	if err != nil {
		return err
	}
	_, err = r.client.ReplyMessage(entry.MessageID, "text", content, entry.RootID, true)
	return err
}

func (r *Runner) buildPrompt(entry inbound.LoggedEvent, resultMaxChars int) string {
	conversationContext, err := r.conversationContext(entry)
	if err != nil {
		r.logger.Printf("failed to fetch conversation context for message_id=%s: %v", entry.MessageID, err)
		conversationContext = "（无法读取飞书话题历史，本次仅使用当前消息。）"
	}
	return buildPromptWithContext(entry, resultMaxChars, conversationContext)
}

func buildPrompt(entry inbound.LoggedEvent, resultMaxChars int) string {
	return buildPromptWithContext(entry, resultMaxChars, formatLoggedFallback(entry))
}

func buildPromptWithContext(entry inbound.LoggedEvent, resultMaxChars int, conversationContext string) string {
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
- root_id: %s
- parent_id: %s
- thread_id: %s

飞书话题上下文（按时间顺序，优先使用同一话题；用于保持连续性）：
%s

用户消息：
%s
`, resultMaxChars, entry.ChatID, entry.SenderOpenID, entry.MessageID, entry.RootID, entry.ParentID, entry.ThreadID, conversationContext, entry.MessageText))
}

func (r *Runner) conversationContext(entry inbound.LoggedEvent) (string, error) {
	containerType := "chat"
	containerID := strings.TrimSpace(entry.ChatID)
	limit := 12

	if strings.TrimSpace(entry.ThreadID) != "" {
		containerType = "thread"
		containerID = strings.TrimSpace(entry.ThreadID)
		limit = 25
	}
	if containerID == "" {
		return formatLoggedFallback(entry), nil
	}

	messages, _, _, err := r.client.ListMessages(containerType, containerID, &api.ListMessagesOptions{
		SortType: "ByCreateTimeDesc",
		PageSize: limit,
	})
	if err != nil {
		return "", err
	}

	items := make([]contextMessage, 0, len(messages)+1)
	seen := map[string]bool{}
	for _, message := range messages {
		item := contextMessageFromAPI(message)
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		items = append(items, item)
		if item.MessageID != "" {
			seen[item.MessageID] = true
		}
	}

	if entry.MessageID != "" && !seen[entry.MessageID] {
		items = append(items, contextMessage{
			MessageID:  entry.MessageID,
			CreateTime: entry.ReceivedAt,
			Sender:     "user",
			Text:       entry.MessageText,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreateTime < items[j].CreateTime
	})

	lines := make([]string, 0, len(items))
	for _, item := range items {
		text := collapseWhitespace(item.Text)
		if text == "" {
			continue
		}
		if len([]rune(text)) > 600 {
			runes := []rune(text)
			text = string(runes[:600]) + "..."
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s: %s", item.CreateTime, item.Sender, text))
	}
	if len(lines) == 0 {
		return formatLoggedFallback(entry), nil
	}
	return strings.Join(lines, "\n"), nil
}

type contextMessage struct {
	MessageID  string
	CreateTime string
	Sender     string
	Text       string
}

func contextMessageFromAPI(message api.Message) contextMessage {
	item := contextMessage{
		MessageID:  message.MessageID,
		CreateTime: message.CreateTime,
		Sender:     "unknown",
		Text:       messageBodyText(message.MsgType, message.Body),
	}
	if message.Sender != nil {
		switch message.Sender.SenderType {
		case "user":
			item.Sender = "user"
		case "app":
			item.Sender = "assistant"
		default:
			item.Sender = message.Sender.SenderType
		}
	}
	return item
}

func messageBodyText(messageType string, body *api.MessageBody) string {
	if body == nil || strings.TrimSpace(body.Content) == "" {
		return ""
	}
	text := inbound.ExtractMessageText(messageType, body.Content)
	if strings.TrimSpace(text) != "" && text != body.Content {
		return text
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(body.Content), &payload); err == nil && strings.TrimSpace(payload.Text) != "" {
		return payload.Text
	}
	return body.Content
}

func formatLoggedFallback(entry inbound.LoggedEvent) string {
	text := collapseWhitespace(entry.MessageText)
	if text == "" {
		return "（没有可用的历史消息。）"
	}
	return fmt.Sprintf("- [%s] user: %s", entry.ReceivedAt, text)
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
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
