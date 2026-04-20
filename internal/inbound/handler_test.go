package inbound

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLoggedEventExtractsText(t *testing.T) {
	entry := NewLoggedEvent(MessageInput{
		Schema:      "2.0",
		EventType:   "im.message.receive_v1",
		MessageID:   "om_123",
		MessageType: "text",
		RawContent:  `{"text":"hello inbound"}`,
	})

	if entry.MessageText != "hello inbound" {
		t.Fatalf("unexpected message text: %s", entry.MessageText)
	}
	if entry.MessageID != "om_123" {
		t.Fatalf("unexpected message_id: %s", entry.MessageID)
	}
	if entry.ReceivedAt == "" {
		t.Fatalf("expected received_at to be populated")
	}
}

func TestHandlerProcessWritesJSONL(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	handler := NewHandler(Config{
		EventLogPath: logPath,
	}, log.New(io.Discard, "", 0))

	entry := NewLoggedEvent(MessageInput{
		EventType:   "im.message.receive_v1",
		MessageID:   "om_123",
		MessageType: "text",
		RawContent:  `{"text":"hello inbound"}`,
	})

	if err := handler.Process(entry); err != nil {
		t.Fatalf("process event: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read event log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var got LoggedEvent
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("unmarshal log entry: %v", err)
	}

	if got.MessageText != "hello inbound" {
		t.Fatalf("unexpected message text: %s", got.MessageText)
	}
}
