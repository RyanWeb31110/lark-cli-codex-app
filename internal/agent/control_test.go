package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yjwong/lark-cli/internal/inbound"
)

func TestControlStatusHidesInternalBindingKeysWhenUnbound(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	runner := NewRunner(Config{
		ThreadBindings: filepath.Join(t.TempDir(), "bindings.json"),
	}, nil)
	entry := inbound.LoggedEvent{
		ChatID:    "oc_123",
		ThreadID:  "omt_123",
		MessageID: "om_123",
	}

	reply := runner.controlStatus(entry)
	if strings.Contains(reply, "thread:") || strings.Contains(reply, "chat:") || strings.Contains(reply, "root:") {
		t.Fatalf("controlStatus leaked internal binding key: %q", reply)
	}
	if !strings.Contains(reply, "#new") {
		t.Fatalf("controlStatus should suggest user-facing commands: %q", reply)
	}
}

func TestControlStatusHidesInternalBindingKeysWhenBound(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	runner := NewRunner(Config{
		ThreadBindings: filepath.Join(t.TempDir(), "bindings.json"),
	}, nil)
	entry := inbound.LoggedEvent{
		ChatID:    "oc_123",
		ThreadID:  "omt_123",
		MessageID: "om_123",
	}
	if _, _, err := runner.bindings.Set(entry, "019edb08-61df-7a41-b685-69c2fc2fa302", true); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if _, _, err := runner.bindings.UpdateActivity(entry, "019edb08-61df-7a41-b685-69c2fc2fa302", "继续处理 Lark bridge", "已修复 #status 文案"); err != nil {
		t.Fatalf("UpdateActivity returned error: %v", err)
	}

	reply := runner.controlStatus(entry)
	if strings.Contains(reply, "thread:") || strings.Contains(reply, "chat:") || strings.Contains(reply, "root:") {
		t.Fatalf("controlStatus leaked internal binding key: %q", reply)
	}
	if !strings.Contains(reply, "会话摘要") || !strings.Contains(reply, "继续处理 Lark bridge") {
		t.Fatalf("controlStatus should include a human-readable summary: %q", reply)
	}
	if !strings.Contains(reply, "019edb08-61df-7a41-b685-69c2fc2fa302") {
		t.Fatalf("controlStatus should include Codex session id: %q", reply)
	}
}

func TestControlStatusExplainsMissingSummary(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	runner := NewRunner(Config{
		ThreadBindings: filepath.Join(t.TempDir(), "bindings.json"),
	}, nil)
	entry := inbound.LoggedEvent{
		ChatID:    "oc_123",
		ThreadID:  "omt_123",
		MessageID: "om_123",
	}
	if _, _, err := runner.bindings.Set(entry, "00000000-0000-4000-8000-000000000001", true); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	reply := runner.controlStatus(entry)
	if !strings.Contains(reply, "暂无摘要") {
		t.Fatalf("controlStatus should explain missing summary: %q", reply)
	}
}
