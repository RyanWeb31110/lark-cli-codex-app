package inbound

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yjwong/lark-cli/internal/api"
)

// Config configures how inbound message events are persisted and handled.
type Config struct {
	EventLogPath  string
	AutoReplyText string
}

// MessageInput is a normalized message event shape that can be populated from
// webhook callbacks or WebSocket events.
type MessageInput struct {
	Schema       string
	EventID      string
	EventType    string
	TenantKey    string
	AppID        string
	MessageID    string
	RootID       string
	ParentID     string
	ChatID       string
	ChatType     string
	MessageType  string
	SenderType   string
	SenderOpenID string
	SenderUserID string
	RawContent   string
	RawEvent     json.RawMessage
}

// LoggedEvent is the JSONL shape persisted by inbound handlers.
type LoggedEvent struct {
	ReceivedAt   string          `json:"received_at"`
	Schema       string          `json:"schema,omitempty"`
	EventID      string          `json:"event_id,omitempty"`
	EventType    string          `json:"event_type,omitempty"`
	TenantKey    string          `json:"tenant_key,omitempty"`
	AppID        string          `json:"app_id,omitempty"`
	MessageID    string          `json:"message_id,omitempty"`
	RootID       string          `json:"root_id,omitempty"`
	ParentID     string          `json:"parent_id,omitempty"`
	ChatID       string          `json:"chat_id,omitempty"`
	ChatType     string          `json:"chat_type,omitempty"`
	MessageType  string          `json:"message_type,omitempty"`
	MessageText  string          `json:"message_text,omitempty"`
	SenderType   string          `json:"sender_type,omitempty"`
	SenderOpenID string          `json:"sender_open_id,omitempty"`
	SenderUserID string          `json:"sender_user_id,omitempty"`
	RawContent   string          `json:"raw_content,omitempty"`
	RawEvent     json.RawMessage `json:"raw_event,omitempty"`
}

// Handler persists inbound events and optionally sends auto replies.
type Handler struct {
	cfg    Config
	client *api.Client
	logger *log.Logger
	mu     sync.Mutex
}

// NewHandler returns a shared inbound handler.
func NewHandler(cfg Config, logger *log.Logger) *Handler {
	if logger == nil {
		logger = log.New(os.Stderr, "lark-inbound: ", log.LstdFlags)
	}
	return &Handler{
		cfg:    cfg,
		client: api.NewClient(),
		logger: logger,
	}
}

// NewLoggedEvent builds a persisted event from a normalized input.
func NewLoggedEvent(input MessageInput) LoggedEvent {
	return LoggedEvent{
		ReceivedAt:   time.Now().Format(time.RFC3339Nano),
		Schema:       input.Schema,
		EventID:      input.EventID,
		EventType:    input.EventType,
		TenantKey:    input.TenantKey,
		AppID:        input.AppID,
		MessageID:    input.MessageID,
		RootID:       input.RootID,
		ParentID:     input.ParentID,
		ChatID:       input.ChatID,
		ChatType:     input.ChatType,
		MessageType:  input.MessageType,
		MessageText:  ExtractMessageText(input.MessageType, input.RawContent),
		SenderType:   input.SenderType,
		SenderOpenID: input.SenderOpenID,
		SenderUserID: input.SenderUserID,
		RawContent:   input.RawContent,
		RawEvent:     input.RawEvent,
	}
}

// Process persists the event and optionally sends an auto reply.
func (h *Handler) Process(entry LoggedEvent) error {
	if entry.ReceivedAt == "" {
		entry.ReceivedAt = time.Now().Format(time.RFC3339Nano)
	}

	if err := h.appendEvent(entry); err != nil {
		return err
	}

	h.logger.Printf(
		"received message event message_id=%s chat_id=%s sender_open_id=%s",
		entry.MessageID,
		entry.ChatID,
		entry.SenderOpenID,
	)

	if h.cfg.AutoReplyText != "" && ShouldAutoReply(entry) {
		if err := h.autoReply(entry); err != nil {
			h.logger.Printf("auto reply failed for message_id=%s: %v", entry.MessageID, err)
		}
	}

	return nil
}

func (h *Handler) appendEvent(entry LoggedEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(h.cfg.EventLogPath), 0700); err != nil {
		return fmt.Errorf("create event log directory: %w", err)
	}

	file, err := os.OpenFile(h.cfg.EventLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	defer file.Close()

	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal event log entry: %w", err)
	}

	if _, err := file.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write event log: %w", err)
	}

	return nil
}

func (h *Handler) autoReply(entry LoggedEvent) error {
	reply := RenderReplyTemplate(h.cfg.AutoReplyText, entry)
	content, err := buildTextContent(reply)
	if err != nil {
		return err
	}

	_, err = h.client.ReplyMessage(entry.MessageID, "text", content, entry.RootID, true)
	return err
}

// ExtractMessageText returns the human-readable text when possible.
func ExtractMessageText(messageType, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	if messageType == "text" {
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			return payload.Text
		}
	}

	var generic map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &generic); err == nil {
		if text, ok := generic["text"].(string); ok {
			return text
		}
	}

	return raw
}

// ShouldAutoReply limits auto replies to user-originated messages with IDs.
func ShouldAutoReply(entry LoggedEvent) bool {
	if entry.MessageID == "" {
		return false
	}
	if entry.SenderType != "" && entry.SenderType != "user" {
		return false
	}
	return true
}

// RenderReplyTemplate fills supported placeholders for auto replies.
func RenderReplyTemplate(template string, entry LoggedEvent) string {
	replacer := strings.NewReplacer(
		"{{text}}", entry.MessageText,
		"{{message_id}}", entry.MessageID,
		"{{chat_id}}", entry.ChatID,
		"{{sender_open_id}}", entry.SenderOpenID,
		"{{sender_user_id}}", entry.SenderUserID,
	)
	return replacer.Replace(template)
}

func buildTextContent(text string) (string, error) {
	payload := map[string]string{"text": text}
	content, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
