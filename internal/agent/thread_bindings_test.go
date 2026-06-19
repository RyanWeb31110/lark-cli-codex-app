package agent

import (
	"path/filepath"
	"testing"

	"github.com/yjwong/lark-cli/internal/inbound"
)

func TestBindingKeysPreferThreadThenRootThenChat(t *testing.T) {
	entry := inbound.LoggedEvent{
		ThreadID:  "omt_123",
		RootID:    "om_root",
		ChatID:    "oc_123",
		MessageID: "om_msg",
	}
	keys := bindingLookupKeys(entry)
	want := []string{"thread:omt_123", "root:om_root", "chat:oc_123"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %#v, want %#v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %#v, want %#v", keys, want)
		}
	}
}

func TestBindingSaveKeysAddTopLevelMessageAsRootAlias(t *testing.T) {
	entry := inbound.LoggedEvent{
		ChatID:    "oc_123",
		MessageID: "om_msg",
	}
	keys := bindingSaveKeys(entry)
	want := []string{"chat:oc_123", "root:om_msg"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %#v, want %#v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %#v, want %#v", keys, want)
		}
	}
}

func TestThreadBindingStoreRoundTripAndRootAlias(t *testing.T) {
	store := NewThreadBindingStore(filepath.Join(t.TempDir(), "bindings.json"))
	topLevel := inbound.LoggedEvent{
		ChatID:    "oc_123",
		MessageID: "om_root",
	}
	if _, _, err := store.Set(topLevel, "thread-123", false); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	reply := inbound.LoggedEvent{
		ChatID:   "oc_123",
		RootID:   "om_root",
		ThreadID: "omt_123",
	}
	binding, matchedKey, ok, err := store.Find(reply)
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected binding")
	}
	if matchedKey != "root:om_root" {
		t.Fatalf("matchedKey = %q, want root alias", matchedKey)
	}
	if binding.CodexThreadID != "thread-123" {
		t.Fatalf("CodexThreadID = %q", binding.CodexThreadID)
	}
}

func TestThreadBindingStoreUpdateActivityPersistsSummary(t *testing.T) {
	store := NewThreadBindingStore(filepath.Join(t.TempDir(), "bindings.json"))
	entry := inbound.LoggedEvent{
		ChatID:    "oc_123",
		ThreadID:  "omt_123",
		MessageID: "om_123",
	}
	if _, _, err := store.Set(entry, "thread-123", false); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if _, _, err := store.UpdateActivity(entry, "thread-123", "检查项目状态", "发现有未提交代码，测试已通过"); err != nil {
		t.Fatalf("UpdateActivity returned error: %v", err)
	}

	binding, _, ok, err := store.Find(entry)
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected binding")
	}
	if binding.Summary == "" {
		t.Fatalf("expected summary to be persisted: %#v", binding)
	}
	if binding.LastUserMessage != "检查项目状态" {
		t.Fatalf("LastUserMessage = %q", binding.LastUserMessage)
	}
	if binding.LastResult != "发现有未提交代码，测试已通过" {
		t.Fatalf("LastResult = %q", binding.LastResult)
	}
	if binding.LastActivityAt == "" {
		t.Fatal("expected LastActivityAt to be populated")
	}
}

func TestParseControlCommand(t *testing.T) {
	cmd, ok := parseControlCommand("#bind 019eda0e-da39-7722")
	if !ok {
		t.Fatal("expected control command")
	}
	if cmd.Name != "bind" || cmd.Arg != "019eda0e-da39-7722" {
		t.Fatalf("cmd = %#v", cmd)
	}
	if _, ok := parseControlCommand("#unknown"); ok {
		t.Fatal("unknown command should not parse")
	}
	if _, ok := parseControlCommand("你好"); ok {
		t.Fatal("plain greeting should not parse as control command")
	}
}
