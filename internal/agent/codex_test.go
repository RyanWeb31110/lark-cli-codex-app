package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yjwong/lark-cli/internal/inbound"
)

func TestTrimForFeishu(t *testing.T) {
	got := trimForFeishu("abcdef", 4)
	if !strings.Contains(got, "[已截断]") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
}

func TestBuildPromptIncludesMessage(t *testing.T) {
	prompt := buildPrompt(inbound.LoggedEvent{
		ChatID:       "oc_123",
		SenderOpenID: "ou_123",
		MessageID:    "om_123",
		MessageText:  "请帮我查看仓库状态",
	}, 1200)

	if !strings.Contains(prompt, "请帮我查看仓库状态") {
		t.Fatalf("prompt did not include message text: %q", prompt)
	}
	if !strings.Contains(prompt, "oc_123") {
		t.Fatalf("prompt did not include chat id: %q", prompt)
	}
}

func TestBuildAppServerInputKeepsVisibleMessageClean(t *testing.T) {
	input := buildAppServerInput(inbound.LoggedEvent{
		ChatID:       "oc_123",
		SenderOpenID: "ou_123",
		MessageID:    "om_123",
		MessageText:  "请帮我查看仓库状态",
	})

	if !strings.Contains(input, "请帮我查看仓库状态") {
		t.Fatalf("input did not include message text: %q", input)
	}
	for _, noisy := range []string{"本地 Codex 执行代理", "chat_id", "sender_open_id", "飞书话题上下文"} {
		if strings.Contains(input, noisy) {
			t.Fatalf("app-server visible input should not include noisy prompt fragment %q: %q", noisy, input)
		}
	}
}

func TestLarkBridgeDeveloperInstructions(t *testing.T) {
	instructions := larkBridgeDeveloperInstructions(1200)
	if !strings.Contains(instructions, "来自飞书消息") {
		t.Fatalf("developer instructions should describe Lark inputs: %q", instructions)
	}
	if !strings.Contains(instructions, "不要承诺") || !strings.Contains(instructions, "实时回显") {
		t.Fatalf("developer instructions should avoid promising live UI echo: %q", instructions)
	}
	if !strings.Contains(instructions, "1200") {
		t.Fatalf("developer instructions should include max result chars: %q", instructions)
	}
}

func TestQuickPresenceReply(t *testing.T) {
	reply, ok := quickPresenceReply(" 在吗？ ")
	if !ok {
		t.Fatal("expected quick presence reply")
	}
	if !strings.Contains(reply, "在的") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestQuickPresenceReplyCatchesChineseGreeting(t *testing.T) {
	reply, ok := quickPresenceReply("你好")
	if !ok {
		t.Fatal("expected quick presence reply")
	}
	if !strings.Contains(reply, "在的") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestQuickPresenceReplyDoesNotCatchTasks(t *testing.T) {
	if _, ok := quickPresenceReply("在吗，帮我看一下仓库状态"); ok {
		t.Fatal("task-like message should not use quick presence reply")
	}
	if _, ok := quickPresenceReply("你好，帮我看一下仓库状态"); ok {
		t.Fatal("task-like greeting should not use quick presence reply")
	}
}

func TestConversationContextTopLevelUsesCurrentMessageOnly(t *testing.T) {
	runner := NewRunner(Config{}, nil)
	got, err := runner.conversationContext(inbound.LoggedEvent{
		ReceivedAt:  "2026-06-18T16:45:31+08:00",
		ChatID:      "oc_123",
		MessageText: "在吗",
	})
	if err != nil {
		t.Fatalf("conversationContext returned error: %v", err)
	}
	if !strings.Contains(got, "在吗") {
		t.Fatalf("context did not include current message: %q", got)
	}
}

func TestBackendNormalization(t *testing.T) {
	tests := map[string]string{
		"":           "app_server",
		"app-server": "app_server",
		"appserver":  "app_server",
		"codex_exec": "exec",
		"codex-exec": "exec",
		"exec":       "exec",
		"unknown":    "app_server",
	}
	for input, want := range tests {
		runner := NewRunner(Config{Backend: input}, nil)
		if got := runner.backend(); got != want {
			t.Fatalf("backend(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveCodexBinaryPrefersBundledAppServerForDefault(t *testing.T) {
	codexHome := t.TempDir()
	bundled := filepath.Join(codexHome, "plugins", ".plugin-appserver", "codex")
	writeExecutableForTest(t, bundled)

	pathDir := t.TempDir()
	pathCodex := filepath.Join(pathDir, "codex")
	writeExecutableForTest(t, pathCodex)

	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("PATH", pathDir)

	got, err := resolveCodexBinary("codex")
	if err != nil {
		t.Fatalf("resolveCodexBinary returned error: %v", err)
	}
	if got != bundled {
		t.Fatalf("resolveCodexBinary() = %q, want bundled %q", got, bundled)
	}
}

func TestResolveCodexBinaryHonorsAbsoluteConfiguredPath(t *testing.T) {
	codexHome := t.TempDir()
	writeExecutableForTest(t, filepath.Join(codexHome, "plugins", ".plugin-appserver", "codex"))

	configured := filepath.Join(t.TempDir(), "custom-codex")
	writeExecutableForTest(t, configured)

	t.Setenv("CODEX_HOME", codexHome)

	got, err := resolveCodexBinary(configured)
	if err != nil {
		t.Fatalf("resolveCodexBinary returned error: %v", err)
	}
	if got != configured {
		t.Fatalf("resolveCodexBinary() = %q, want configured %q", got, configured)
	}
}

func TestCompletedAgentMessageFromItem(t *testing.T) {
	params := json.RawMessage(`{"threadId":"thread-1","item":{"type":"agentMessage","text":"done"}}`)
	if got := completedAgentMessage(params); got != "done" {
		t.Fatalf("completedAgentMessage() = %q, want done", got)
	}
}

func TestTurnCompletionError(t *testing.T) {
	completed := json.RawMessage(`{"turn":{"status":"completed"}}`)
	if err := turnCompletionError(completed); err != nil {
		t.Fatalf("completed turn returned error: %v", err)
	}

	failed := json.RawMessage(`{"turn":{"status":"failed","error":{"message":"network failed","additionalDetails":"retry later"}}}`)
	err := turnCompletionError(failed)
	if err == nil {
		t.Fatal("failed turn did not return error")
	}
	if !strings.Contains(err.Error(), "network failed") || !strings.Contains(err.Error(), "retry later") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMessageThreadMatches(t *testing.T) {
	params := json.RawMessage(`{"threadId":"thread-1"}`)
	if !messageThreadMatches(params, "thread-1") {
		t.Fatal("expected thread id to match")
	}
	if messageThreadMatches(params, "thread-2") {
		t.Fatal("different thread id should not match")
	}
}

func writeExecutableForTest(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
