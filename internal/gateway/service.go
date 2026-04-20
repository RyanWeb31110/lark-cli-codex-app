package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/yjwong/lark-cli/internal/config"
	"github.com/yjwong/lark-cli/internal/inbound"
)

// Config configures the local WebSocket gateway.
type Config struct {
	EventLogPath  string
	AutoReplyText string
}

// Service receives Feishu/Lark events through the long-connection WebSocket SDK.
type Service struct {
	cfg     Config
	logger  *log.Logger
	handler *inbound.Handler
}

// New returns a WebSocket gateway service.
func New(cfg Config) *Service {
	logger := log.New(os.Stderr, "lark-gateway: ", log.LstdFlags)
	return &Service{
		cfg:     cfg,
		logger:  logger,
		handler: inbound.NewHandler(inbound.Config(cfg), logger),
	}
}

// Serve starts the long-connection client and waits until the context is canceled
// or the SDK returns an error.
func (s *Service) Serve(ctx context.Context) error {
	appID := strings.TrimSpace(config.GetAppID())
	if appID == "" {
		return fmt.Errorf("app_id is required")
	}

	appSecret := strings.TrimSpace(config.GetAppSecret())
	if appSecret == "" {
		return fmt.Errorf("LARK_APP_SECRET is required")
	}

	dispatcher := larkdispatcher.NewEventDispatcher("", "")
	dispatcher.OnP2MessageReceiveV1(s.handleMessageReceive)

	opts := []larkws.ClientOption{
		larkws.WithEventHandler(dispatcher),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	}
	if config.GetRegion() == "lark" {
		opts = append(opts, larkws.WithDomain(lark.LarkBaseUrl))
	} else {
		opts = append(opts, larkws.WithDomain(lark.FeishuBaseUrl))
	}

	client := larkws.NewClient(appID, appSecret, opts...)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Start(ctx)
	}()

	select {
	case <-ctx.Done():
		s.logger.Printf("gateway shutdown requested")
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Service) handleMessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	entry, err := buildLoggedEvent(event)
	if err != nil {
		return err
	}
	return s.handler.Process(entry)
}

func buildLoggedEvent(event *larkim.P2MessageReceiveV1) (inbound.LoggedEvent, error) {
	if event == nil {
		return inbound.LoggedEvent{}, fmt.Errorf("message event is nil")
	}

	raw := json.RawMessage{}
	if event.EventReq != nil && len(event.EventReq.Body) > 0 {
		raw = append(raw, event.EventReq.Body...)
	} else {
		payload, err := json.Marshal(event)
		if err != nil {
			return inbound.LoggedEvent{}, fmt.Errorf("marshal websocket event: %w", err)
		}
		raw = payload
	}

	input := inbound.MessageInput{
		RawEvent: raw,
	}
	if event.EventV2Base != nil {
		input.Schema = event.Schema
		if event.EventV2Base.Header != nil {
			input.EventID = event.EventV2Base.Header.EventID
			input.EventType = event.EventV2Base.Header.EventType
			input.TenantKey = event.EventV2Base.Header.TenantKey
			input.AppID = event.EventV2Base.Header.AppID
		}
	}
	if event.Event != nil {
		if event.Event.Message != nil {
			input.MessageID = stringValue(event.Event.Message.MessageId)
			input.RootID = stringValue(event.Event.Message.RootId)
			input.ParentID = stringValue(event.Event.Message.ParentId)
			input.ChatID = stringValue(event.Event.Message.ChatId)
			input.ChatType = stringValue(event.Event.Message.ChatType)
			input.MessageType = stringValue(event.Event.Message.MessageType)
			input.RawContent = stringValue(event.Event.Message.Content)
		}
		if event.Event.Sender != nil {
			input.SenderType = stringValue(event.Event.Sender.SenderType)
			if event.Event.Sender.SenderId != nil {
				input.SenderOpenID = stringValue(event.Event.Sender.SenderId.OpenId)
				input.SenderUserID = stringValue(event.Event.Sender.SenderId.UserId)
			}
		}
	}

	return inbound.NewLoggedEvent(input), nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
