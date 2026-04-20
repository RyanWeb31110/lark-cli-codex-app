package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yjwong/lark-cli/internal/inbound"
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
	cfg     Config
	logger  *log.Logger
	handler *inbound.Handler
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

// LoggedEvent is the JSONL shape persisted by inbound handlers.
type LoggedEvent = inbound.LoggedEvent

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
	logger := log.New(os.Stderr, "lark-webhook: ", log.LstdFlags)
	return &Server{
		cfg:    cfg,
		logger: logger,
		handler: inbound.NewHandler(inbound.Config{
			EventLogPath:  cfg.EventLogPath,
			AutoReplyText: cfg.AutoReplyText,
		}, logger),
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
		if err := s.handler.Process(entry); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "append_failed",
				"message": err.Error(),
			})
			return
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

func buildLoggedEvent(envelope CallbackEnvelope, raw json.RawMessage) LoggedEvent {
	input := inbound.MessageInput{
		Schema:   envelope.Schema,
		RawEvent: raw,
	}
	if envelope.Header != nil {
		input.EventID = envelope.Header.EventID
		input.EventType = envelope.Header.EventType
		input.TenantKey = envelope.Header.TenantKey
		input.AppID = envelope.Header.AppID
	}
	if envelope.Event != nil {
		input.MessageID = envelope.Event.Message.MessageID
		input.RootID = envelope.Event.Message.RootID
		input.ParentID = envelope.Event.Message.ParentID
		input.ChatID = envelope.Event.Message.ChatID
		input.ChatType = envelope.Event.Message.ChatType
		input.MessageType = envelope.Event.Message.MessageType
		input.RawContent = envelope.Event.Message.Content
		input.SenderType = envelope.Event.Sender.SenderType
		input.SenderOpenID = envelope.Event.Sender.SenderID.OpenID
		input.SenderUserID = envelope.Event.Sender.SenderID.UserID
	}
	return inbound.NewLoggedEvent(input)
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
