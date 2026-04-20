package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yjwong/lark-cli/internal/api"
)

// Config configures the webhook server.
type Config struct {
	ListenAddr        string
	Path              string
	VerificationToken string
	EventLogPath      string
	AutoReplyText     string
}

// Server handles Feishu event callbacks.
type Server struct {
	cfg    Config
	client *api.Client
	logger *log.Logger
	mu     sync.Mutex
}

// CallbackEnvelope covers the Feishu URL verification and event callback shapes used by this server.
type CallbackEnvelope struct {
	Schema    string        `json:"schema,omitempty"`
	Type      string        `json:"type,omitempty"`
	Token     string        `json:"token,omitempty"`
	Challenge string        `json:"challenge,omitempty"`
	Encrypt   string        `json:"encrypt,omitempty"`
	Header    *EventHeader  `json:"header,omitempty"`
	Event     *MessageEvent `json:"event,omitempty"`
}

// EventHeader carries callback metadata.
type EventHeader struct {
	EventID    string `json:"event_id,omitempty"`
	EventType  string `json:"event_type,omitempty"`
	CreateTime string `json:"create_time,omitempty"`
	Token      string `json:"token,omitempty"`
	TenantKey  string `json:"tenant_key,omitempty"`
	AppID      string `json:"app_id,omitempty"`
}

// MessageEvent contains the fields we need from im.message.receive_v1 callbacks.
type MessageEvent struct {
	Sender  EventSender  `json:"sender"`
	Message EventMessage `json:"message"`
}

// EventSender describes the sender of the incoming message.
type EventSender struct {
	SenderID   SenderID `json:"sender_id"`
	SenderType string   `json:"sender_type"`
	TenantKey  string   `json:"tenant_key,omitempty"`
}

// SenderID holds multiple user identifiers.
type SenderID struct {
	OpenID  string `json:"open_id,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	UnionID string `json:"union_id,omitempty"`
}

// EventMessage contains the subset of message fields we need for logging and replies.
type EventMessage struct {
	MessageID   string `json:"message_id"`
	RootID      string `json:"root_id,omitempty"`
	ParentID    string `json:"parent_id,omitempty"`
	CreateTime  string `json:"create_time,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
	ChatType    string `json:"chat_type,omitempty"`
	MessageType string `json:"message_type,omitempty"`
	Content     string `json:"content,omitempty"`
}

// LoggedEvent is the JSONL shape persisted by the webhook server.
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

// New returns a webhook server.
func New(cfg Config) *Server {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "0.0.0.0:8080"
	}
	if cfg.Path == "" {
		cfg.Path = "/webhook/feishu"
	}
	if !strings.HasPrefix(cfg.Path, "/") {
		cfg.Path = "/" + cfg.Path
	}
	if cfg.EventLogPath == "" {
		cfg.EventLogPath = "webhook-events.jsonl"
	}
	return &Server{
		cfg:    cfg,
		client: api.NewClient(),
		logger: log.New(os.Stderr, "lark-webhook: ", log.LstdFlags),
	}
}

// Serve starts the webhook server and blocks until ctx is canceled or the server exits.
func (s *Server) Serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.Path, s.handleCallback)
	mux.HandleFunc("/healthz", s.handleHealth)

	srv := &http.Server{
		Addr:    s.cfg.ListenAddr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Printf("listening on %s%s", s.cfg.ListenAddr, s.cfg.Path)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"listen":     s.cfg.ListenAddr,
		"path":       s.cfg.Path,
		"event_log":  s.cfg.EventLogPath,
		"auto_reply": s.cfg.AutoReplyText != "",
	})
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "method_not_allowed",
		})
		return
	}

	defer r.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "invalid_json",
			"message": err.Error(),
		})
		return
	}

	var envelope CallbackEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "invalid_payload",
			"message": err.Error(),
		})
		return
	}

	if envelope.Encrypt != "" {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error":   "encrypted_events_not_supported",
			"message": "clear the Encrypt Key in Feishu event subscriptions for this version",
		})
		return
	}

	if envelope.Type == "url_verification" {
		if err := s.validateToken(envelope.Token, ""); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error":   "invalid_token",
				"message": err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"challenge": envelope.Challenge,
		})
		return
	}

	headerToken := ""
	if envelope.Header != nil {
		headerToken = envelope.Header.Token
	}
	if err := s.validateToken(envelope.Token, headerToken); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error":   "invalid_token",
			"message": err.Error(),
		})
		return
	}

	if envelope.Header != nil && envelope.Header.EventType == "im.message.receive_v1" && envelope.Event != nil {
		entry := buildLoggedEvent(envelope, raw)
		if err := s.appendEvent(entry); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "append_failed",
				"message": err.Error(),
			})
			return
		}
		s.logger.Printf("received message event message_id=%s chat_id=%s sender_open_id=%s", entry.MessageID, entry.ChatID, entry.SenderOpenID)
		if s.cfg.AutoReplyText != "" && shouldAutoReply(entry) {
			if err := s.autoReply(entry); err != nil {
				s.logger.Printf("auto reply failed for message_id=%s: %v", entry.MessageID, err)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{
		"code": 0,
	})
}

func (s *Server) validateToken(tokens ...string) error {
	if strings.TrimSpace(s.cfg.VerificationToken) == "" {
		return nil
	}
	for _, token := range tokens {
		if strings.TrimSpace(token) == s.cfg.VerificationToken {
			return nil
		}
	}
	return fmt.Errorf("callback token did not match configured webhook.verification_token")
}

func (s *Server) appendEvent(entry LoggedEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.cfg.EventLogPath), 0700); err != nil {
		return fmt.Errorf("create event log directory: %w", err)
	}

	file, err := os.OpenFile(s.cfg.EventLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
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

func (s *Server) autoReply(entry LoggedEvent) error {
	reply := renderReplyTemplate(s.cfg.AutoReplyText, entry)
	content, err := buildTextContent(reply)
	if err != nil {
		return err
	}

	_, err = s.client.ReplyMessage(entry.MessageID, "text", content, entry.RootID, true)
	return err
}

func buildLoggedEvent(envelope CallbackEnvelope, raw json.RawMessage) LoggedEvent {
	entry := LoggedEvent{
		ReceivedAt: time.Now().Format(time.RFC3339Nano),
		Schema:     envelope.Schema,
		RawEvent:   raw,
	}
	if envelope.Header != nil {
		entry.EventID = envelope.Header.EventID
		entry.EventType = envelope.Header.EventType
		entry.TenantKey = envelope.Header.TenantKey
		entry.AppID = envelope.Header.AppID
	}
	if envelope.Event != nil {
		entry.MessageID = envelope.Event.Message.MessageID
		entry.RootID = envelope.Event.Message.RootID
		entry.ParentID = envelope.Event.Message.ParentID
		entry.ChatID = envelope.Event.Message.ChatID
		entry.ChatType = envelope.Event.Message.ChatType
		entry.MessageType = envelope.Event.Message.MessageType
		entry.RawContent = envelope.Event.Message.Content
		entry.MessageText = extractMessageText(envelope.Event.Message.MessageType, envelope.Event.Message.Content)
		entry.SenderType = envelope.Event.Sender.SenderType
		entry.SenderOpenID = envelope.Event.Sender.SenderID.OpenID
		entry.SenderUserID = envelope.Event.Sender.SenderID.UserID
	}
	return entry
}

func extractMessageText(messageType, raw string) string {
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

func shouldAutoReply(entry LoggedEvent) bool {
	if entry.MessageID == "" {
		return false
	}
	if entry.SenderType != "" && entry.SenderType != "user" {
		return false
	}
	return true
}

func renderReplyTemplate(template string, entry LoggedEvent) string {
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

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
