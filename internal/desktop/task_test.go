package desktop

import (
	"path/filepath"
	"testing"

	"github.com/yjwong/lark-cli/internal/inbound"
)

func TestExtractRequest(t *testing.T) {
	request, ok := ExtractRequest("/gui 打开 Safari")
	if !ok || request != "打开 Safari" {
		t.Fatalf("unexpected parse result: ok=%v request=%q", ok, request)
	}
}

func TestExtractRequestAutoDetectsDesktopIntent(t *testing.T) {
	request, ok := ExtractRequest("打开 Safari，然后访问 openai.com")
	if !ok || request != "打开 Safari，然后访问 openai.com" {
		t.Fatalf("unexpected desktop detection result: ok=%v request=%q", ok, request)
	}
}

func TestExtractRequestAutoDetectsUiAction(t *testing.T) {
	request, ok := ExtractRequest("点击发送按钮")
	if !ok || request != "点击发送按钮" {
		t.Fatalf("unexpected UI action detection result: ok=%v request=%q", ok, request)
	}
}

func TestExtractRequestIgnoresCapabilityQuestion(t *testing.T) {
	request, ok := ExtractRequest("我能通过你进行 computer use 操作吗？")
	if ok || request != "" {
		t.Fatalf("unexpected capability routing result: ok=%v request=%q", ok, request)
	}
}

func TestExtractRequestIgnoresTerminalTask(t *testing.T) {
	request, ok := ExtractRequest("到 /Users/ryan/WorkSpace 看一下 git status")
	if ok || request != "" {
		t.Fatalf("unexpected terminal routing result: ok=%v request=%q", ok, request)
	}
}

func TestQueueLifecycle(t *testing.T) {
	queue := NewQueue(filepath.Join(t.TempDir(), "desktop-tasks"))
	entry := inbound.LoggedEvent{
		MessageID:    "om_123",
		ChatID:       "oc_123",
		SenderOpenID: "ou_123",
	}

	task, err := queue.Enqueue(entry, "打开 Safari")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if task.Status != statusPending {
		t.Fatalf("unexpected pending status: %s", task.Status)
	}

	popped, err := queue.PopPending()
	if err != nil {
		t.Fatalf("pop pending: %v", err)
	}
	if popped == nil || popped.ID != task.ID {
		t.Fatalf("unexpected popped task: %#v", popped)
	}
	if popped.Status != statusProcessing {
		t.Fatalf("unexpected processing status: %s", popped.Status)
	}

	completed, err := queue.Complete(task.ID, "done", false)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Status != statusCompleted {
		t.Fatalf("unexpected completed status: %s", completed.Status)
	}
}
