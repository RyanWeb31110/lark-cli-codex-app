package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yjwong/lark-cli/internal/inbound"
)

type controlCommand struct {
	Name string
	Arg  string
}

func parseControlCommand(text string) (controlCommand, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return controlCommand{}, false
	}
	if strings.HasPrefix(text, "＃") {
		text = "#" + strings.TrimPrefix(text, "＃")
	}
	if !strings.HasPrefix(text, "#") {
		return controlCommand{}, false
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return controlCommand{}, false
	}
	name := strings.ToLower(strings.TrimPrefix(fields[0], "#"))
	switch name {
	case "status", "bind", "new", "reset":
	default:
		return controlCommand{}, false
	}
	arg := ""
	if len(fields) > 1 {
		arg = strings.TrimSpace(strings.Join(fields[1:], " "))
	}
	return controlCommand{Name: name, Arg: arg}, true
}

func (r *Runner) handleControlCommand(entry inbound.LoggedEvent, cmd controlCommand) string {
	switch cmd.Name {
	case "status":
		return r.controlStatus(entry)
	case "bind":
		return r.controlBind(entry, cmd.Arg)
	case "new":
		return r.controlNew(entry)
	case "reset":
		return r.controlReset(entry)
	default:
		return ""
	}
}

func (r *Runner) controlStatus(entry inbound.LoggedEvent) string {
	if primaryBindingKey(entry) == "" {
		return "这条飞书消息缺少会话信息，暂时无法连接到 Codex 会话。"
	}
	binding, _, ok, err := r.bindings.Find(entry)
	if err != nil {
		return "读取 Codex 会话连接状态失败：" + trimForFeishu(err.Error(), r.cfg.ResultMaxChars)
	}
	if !ok {
		return "当前飞书话题还没有连接到 Codex 会话。\n\n" + r.formatRecentSessionChoices()
	}
	session, hasSession := r.findSessionSummary(binding.CodexThreadID)
	return fmt.Sprintf("当前飞书话题已连接到 Codex 会话。\n\n会话摘要：\n%s\n\nCodex 会话 ID：%s\n更新时间：%s\n\n之后在这个飞书话题里发普通消息，会继续这个 Codex 会话。", bindingSummaryForStatus(binding, session, hasSession), binding.CodexThreadID, bindingStatusTime(binding, session, hasSession))
}

func (r *Runner) controlBind(entry inbound.LoggedEvent, threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return "用法：#bind 1 或 #bind <codex_thread_id>\n\n" + r.formatRecentSessionChoices()
	}
	if r.backend() != "app_server" || r.appServer == nil {
		return "当前桥服务没有启用 Codex app-server 模式，暂时不能连接已有 Codex 会话。"
	}

	selectedSession, hasSelectedSession, err := r.resolveBindSession(threadID)
	if err != nil {
		return "读取最近 Codex 会话失败：" + trimForFeishu(err.Error(), r.cfg.ResultMaxChars)
	}
	if isPositiveInteger(threadID) {
		if !hasSelectedSession {
			return "没有找到这个编号对应的 Codex 会话。\n\n" + r.formatRecentSessionChoices()
		}
		threadID = selectedSession.ID
	} else if session, ok := r.findSessionSummary(threadID); ok {
		selectedSession = session
		hasSelectedSession = true
	}

	ctx, cancel := context.WithTimeout(context.Background(), controlTimeout(r.cfg.Timeout))
	defer cancel()

	if err := r.appServer.ResumeThread(ctx, threadID); err != nil {
		return "连接失败：Codex app-server 无法打开这个会话。\n" + trimForFeishu(err.Error(), r.cfg.ResultMaxChars)
	}
	binding, _, err := r.bindings.Set(entry, threadID, true)
	if err != nil {
		return "保存连接状态失败：" + trimForFeishu(err.Error(), r.cfg.ResultMaxChars)
	}
	return fmt.Sprintf("已把当前飞书话题连接到 Codex 会话。\n\n会话摘要：\n%s\n\nCodex 会话 ID：%s", sessionSummaryForBind(selectedSession, hasSelectedSession), binding.CodexThreadID)
}

func (r *Runner) controlNew(entry inbound.LoggedEvent) string {
	if r.backend() != "app_server" || r.appServer == nil {
		return "当前桥服务没有启用 Codex app-server 模式，暂时不能新建 Codex 会话。"
	}
	ctx, cancel := context.WithTimeout(context.Background(), controlTimeout(r.cfg.Timeout))
	defer cancel()

	threadID, err := r.appServer.NewThread(ctx)
	if err != nil {
		return "新建 Codex 会话失败：" + trimForFeishu(err.Error(), r.cfg.ResultMaxChars)
	}
	binding, _, err := r.bindings.Set(entry, threadID, false)
	if err != nil {
		return "保存连接状态失败：" + trimForFeishu(err.Error(), r.cfg.ResultMaxChars)
	}
	return fmt.Sprintf("已新建并连接 Codex 会话。\n\n会话摘要：\n暂无摘要。发第一条任务后，我会自动记录这个会话在做什么。\n\nCodex 会话 ID：%s", binding.CodexThreadID)
}

func (r *Runner) controlReset(entry inbound.LoggedEvent) string {
	_, deleted, err := r.bindings.Delete(entry)
	if err != nil {
		return "断开 Codex 会话连接失败：" + trimForFeishu(err.Error(), r.cfg.ResultMaxChars)
	}
	if !deleted {
		return "当前飞书话题还没有连接 Codex 会话，无需断开。"
	}
	return "已断开当前飞书话题和 Codex 会话的连接。"
}

func controlTimeout(configured time.Duration) time.Duration {
	if configured > 0 && configured < 2*time.Minute {
		return configured
	}
	return 2 * time.Minute
}

func bindingSummaryForStatus(binding ThreadBinding, session codexSessionSummary, hasSession bool) string {
	if strings.TrimSpace(binding.Summary) != "" {
		return binding.Summary
	}
	if hasSession && strings.TrimSpace(session.Summary) != "" {
		return session.Summary
	}
	if strings.TrimSpace(binding.LastUserMessage) != "" {
		return "最近在处理：" + binding.LastUserMessage
	}
	if strings.TrimSpace(binding.LastResult) != "" {
		return "最近结果：" + binding.LastResult
	}
	return "暂无摘要。这个会话还没有通过飞书处理过任务；发一条任务后，我会自动记录它在做什么。"
}

func bindingStatusTime(binding ThreadBinding, session codexSessionSummary, hasSession bool) string {
	if strings.TrimSpace(binding.LastActivityAt) != "" {
		return binding.LastActivityAt
	}
	if hasSession && !session.UpdatedAtTime.IsZero() {
		return session.UpdatedAtTime.Local().Format(time.RFC3339)
	}
	return binding.UpdatedAt
}

func (r *Runner) resolveBindSession(arg string) (codexSessionSummary, bool, error) {
	if !isPositiveInteger(arg) {
		return codexSessionSummary{}, false, nil
	}
	return defaultCodexSessionCatalog().ResolveOrdinal(arg, 5)
}

func (r *Runner) findSessionSummary(threadID string) (codexSessionSummary, bool) {
	session, ok, err := defaultCodexSessionCatalog().FindByID(threadID)
	if err != nil {
		r.logger.Printf("failed to read Codex session summary for thread_id=%s: %v", threadID, err)
		return codexSessionSummary{}, false
	}
	return session, ok
}

func (r *Runner) formatRecentSessionChoices() string {
	sessions, err := defaultCodexSessionCatalog().Recent(5)
	if err != nil {
		r.logger.Printf("failed to list recent Codex sessions: %v", err)
		return "读取最近 Codex 会话失败。也可以发送：#new 新建会话。"
	}
	if len(sessions) == 0 {
		return "没有找到可连接的本机 Codex 会话。\n\n发送 #new 新建一个会话。"
	}

	lines := []string{"最近可连接的 Codex 会话："}
	for i, session := range sessions {
		project := formatSessionProject(session)
		if project != "" {
			project = "，项目：" + project
		}
		lines = append(lines, fmt.Sprintf("%d. %s（%s%s）", i+1, session.Title, formatSessionChoiceTime(session), project))
		lines = append(lines, "   "+previewText(session.Summary, 120))
	}
	lines = append(lines, "")
	lines = append(lines, "发送 #bind 1 连接第 1 个，或发送 #new 新建会话。")
	return strings.Join(lines, "\n")
}

func sessionSummaryForBind(session codexSessionSummary, ok bool) string {
	if ok && strings.TrimSpace(session.Summary) != "" {
		return session.Summary
	}
	return "暂无本地摘要。等你在飞书里发一条任务后，我会自动更新摘要。"
}

func isPositiveInteger(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
