package agent

import (
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
