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
	Enabled         bool
	Backend         string
	CodexBinary     string
	Workspace       string
	Model           string
	ReasoningEffort string
	ThreadBindings  string
	AckText         string
	ResultMaxChars  int
	Timeout         time.Duration
}

type Runner struct {
	cfg       Config
	client    *api.Client
	logger    *log.Logger
	appServer *appServerClient
	bindings  *ThreadBindingStore
}

func NewRunner(cfg Config, logger *log.Logger) *Runner {
	if logger == nil {
		logger = log.New(os.Stderr, "lark-agent: ", log.LstdFlags)
	}
	if cfg.CodexBinary == "" {
		cfg.CodexBinary = "codex"
	}
	if strings.TrimSpace(cfg.Backend) == "" {
		cfg.Backend = "app_server"
	}
	if strings.TrimSpace(cfg.ThreadBindings) == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			cfg.ThreadBindings = filepath.Join(home, ".lark", "codex-thread-bindings.json")
		}
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

	runner := &Runner{
		cfg:      cfg,
		client:   api.NewClient(),
		logger:   logger,
		bindings: NewThreadBindingStore(cfg.ThreadBindings),
	}
	if runner.backend() == "app_server" {
		runner.appServer = newAppServerClient(cfg, logger)
	}
	return runner
}

func (r *Runner) Enabled() bool {
	return r != nil && r.cfg.Enabled
}

func (r *Runner) Close() error {
	if r == nil || r.appServer == nil {
		return nil
	}
	return r.appServer.Close()
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
	if cmd, ok := parseControlCommand(entry.MessageText); ok {
		reply := r.handleControlCommand(entry, cmd)
		if strings.TrimSpace(reply) == "" {
			reply = "未知控制命令。可用命令：#status、#bind <thread_id>、#new、#reset。"
		}
		if err := r.reply(entry, reply); err != nil {
			r.logger.Printf("failed to send control reply for message_id=%s: %v", entry.MessageID, err)
		}
		return
	}

	if reply, ok := quickPresenceReply(entry.MessageText); ok {
		if err := r.reply(entry, reply); err != nil {
			r.logger.Printf("failed to send presence reply for message_id=%s: %v", entry.MessageID, err)
		}
		r.logger.Printf("quick presence reply sent for message_id=%s text=%q", entry.MessageID, trimForFeishu(entry.MessageText, 60))
		return
	}

	if err := r.reply(entry, r.cfg.AckText); err != nil {
		r.logger.Printf("failed to send ack for message_id=%s: %v", entry.MessageID, err)
	}

	startedAt := time.Now()
	result, err := r.execute(entry)
	if err != nil {
		r.logger.Printf("codex task failed for message_id=%s: %v", entry.MessageID, err)
		result = "处理失败：" + trimForFeishu(err.Error(), r.cfg.ResultMaxChars)
	} else {
		r.logger.Printf("codex task completed for message_id=%s backend=%s duration=%s", entry.MessageID, r.backend(), time.Since(startedAt).Round(time.Millisecond))
	}

	if err := r.reply(entry, result); err != nil {
		r.logger.Printf("failed to send final reply for message_id=%s: %v", entry.MessageID, err)
	}
}

func (r *Runner) execute(entry inbound.LoggedEvent) (string, error) {
	if r.backend() == "app_server" {
		ctx, cancel := context.WithTimeout(context.Background(), r.cfg.Timeout)
		defer cancel()
		binding, matchedKey, ok, err := r.bindings.Find(entry)
		if err != nil {
			return "", err
		}
		threadID := ""
		if ok {
			threadID = binding.CodexThreadID
			r.logger.Printf("using bound Codex thread for message_id=%s matched_key=%s thread_id=%s", entry.MessageID, matchedKey, threadID)
		}
		result, usedThreadID, err := r.appServer.Execute(ctx, buildAppServerInput(entry), threadID)
		if err != nil {
			return "", err
		}
		if usedThreadID != "" {
			if _, keys, err := r.bindings.Set(entry, usedThreadID, ok && binding.Manual); err != nil {
				r.logger.Printf("failed to persist Codex thread binding for message_id=%s thread_id=%s: %v", entry.MessageID, usedThreadID, err)
			} else {
				r.logger.Printf("persisted Codex thread binding for message_id=%s thread_id=%s keys=%s", entry.MessageID, usedThreadID, strings.Join(keys, ","))
			}
			if _, keys, err := r.bindings.UpdateActivity(entry, usedThreadID, entry.MessageText, result); err != nil {
				r.logger.Printf("failed to update Codex thread summary for message_id=%s thread_id=%s: %v", entry.MessageID, usedThreadID, err)
			} else {
				r.logger.Printf("updated Codex thread summary for message_id=%s thread_id=%s keys=%s", entry.MessageID, usedThreadID, strings.Join(keys, ","))
			}
		}
		return result, nil
	}
	prompt := r.buildPrompt(entry, r.cfg.ResultMaxChars)
	return r.executeCodexExec(prompt)
}

func (r *Runner) backend() string {
	switch strings.ToLower(strings.TrimSpace(r.cfg.Backend)) {
	case "app-server", "app_server", "appserver":
		return "app_server"
	case "exec", "codex_exec", "codex-exec":
		return "exec"
	default:
		return "app_server"
	}
}

func (r *Runner) executeCodexExec(prompt string) (string, error) {
	tempDir, err := os.MkdirTemp("", "lark-codex-agent-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	outputFile := filepath.Join(tempDir, "last-message.txt")

	args := []string{
		"-a", "never",
		"-s", "workspace-write",
		"exec",
		"-C", r.cfg.Workspace,
		"--skip-git-repo-check",
		"--ephemeral",
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

func buildAppServerInput(entry inbound.LoggedEvent) string {
	text := strings.TrimSpace(entry.MessageText)
	if text == "" {
		return ""
	}
	return "来自飞书消息（通过本地 bridge 写入底层 Codex thread；Codex App 当前窗口可能不会实时回显）：\n" + text
}

func larkBridgeDeveloperInstructions(resultMaxChars int) string {
	if resultMaxChars <= 0 {
		resultMaxChars = 1800
	}
	return strings.TrimSpace(fmt.Sprintf(`
这条 Codex thread 可能由本地 Lark/Feishu bridge 触发。

当用户输入以“来自飞书消息：”开头时：
- 直接处理用户请求，尽量少讲方案、多做事。
- 默认使用中文回复，输出要适合直接发回飞书。
- 如果任务能执行，就执行后汇报结果。
- 如果缺少关键信息或存在明显风险，只说最关键的阻塞点。
- 不要承诺或暗示 Codex App 当前聊天窗口会实时回显；bridge 只能写入底层 Codex thread，UI 刷新取决于 Codex App 自身。
- 最终回复尽量控制在 %d 个字符以内。
`, resultMaxChars))
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
	threadID := strings.TrimSpace(entry.ThreadID)
	if threadID == "" {
		return formatLoggedFallback(entry), nil
	}

	messages, _, _, err := r.client.ListMessages("thread", threadID, &api.ListMessagesOptions{
		SortType: "ByCreateTimeDesc",
		PageSize: 25,
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

func quickPresenceReply(text string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = strings.Trim(normalized, " \t\r\n。？！?!,.，、~～")
	normalized = collapseWhitespace(normalized)
	switch normalized {
	case "在", "在吗", "在么", "在不在", "你在吗", "还在吗",
		"你好", "您好", "哈喽", "哈啰", "嗨", "早", "早上好", "下午好", "晚上好",
		"hello", "hi", "hey", "ping", "test", "测试":
		return "在的，我在。有什么需要我处理的，直接发我就行。", true
	default:
		return "", false
	}
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
