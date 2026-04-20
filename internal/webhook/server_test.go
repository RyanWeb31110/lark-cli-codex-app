package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yjwong/lark-cli/internal/inbound"
)

func TestHandleURLVerification(t *testing.T) {
	srv := New(Config{
		Path:              "/webhook/feishu",
		VerificationToken: "test-token",
		EventLogPath:      filepath.Join(t.TempDir(), "events.jsonl"),
	})

	body := `{"type":"url_verification","token":"test-token","challenge":"hello-challenge"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	srv.handleCallback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if payload["challenge"] != "hello-challenge" {
		t.Fatalf("expected challenge response, got %#v", payload)
	}
}

func TestHandleMessageEventWritesJSONL(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	srv := New(Config{
		Path:              "/webhook/feishu",
		VerificationToken: "test-token",
		EventLogPath:      logPath,
	})

	body := `{
		"schema":"2.0",
		"header":{
			"event_id":"evt_123",
			"event_type":"im.message.receive_v1",
			"token":"test-token",
			"tenant_key":"tenant_123",
			"app_id":"cli_test"
		},
		"event":{
			"sender":{
				"sender_id":{"open_id":"ou_123","user_id":"u_123"},
				"sender_type":"user"
			},
			"message":{
				"message_id":"om_123",
				"chat_id":"oc_123",
				"chat_type":"group",
				"message_type":"text",
				"content":"{\"text\":\"hello webhook\"}"
			}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	srv.handleCallback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read event log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var entry inbound.LoggedEvent
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("unmarshal log entry: %v", err)
	}

	if entry.EventType != "im.message.receive_v1" {
		t.Fatalf("unexpected event type: %s", entry.EventType)
	}
	if entry.MessageID != "om_123" {
		t.Fatalf("unexpected message_id: %s", entry.MessageID)
	}
	if entry.MessageText != "hello webhook" {
		t.Fatalf("unexpected message text: %s", entry.MessageText)
	}
}
