package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/yjwong/lark-cli/internal/agent"
	"github.com/yjwong/lark-cli/internal/config"
	"github.com/yjwong/lark-cli/internal/desktop"
	"github.com/yjwong/lark-cli/internal/inbound"
)

// Config configures the local WebSocket gateway.
type Config struct {
	EventLogPath  string
	AutoReplyText string
	Agent         agent.Config
	DesktopWorker bool
}

// Service receives Feishu/Lark events through the long-connection WebSocket SDK.
type Service struct {
	cfg     Config
	logger  *log.Logger
	handler *inbound.Handler
	agent   *agent.Runner
	desktop *desktop.Queue
	worker  *desktop.Worker
}

// New returns a WebSocket gateway service.
func New(cfg Config) *Service {
	logger := log.New(os.Stderr, "lark-gateway: ", log.LstdFlags)
	return &Service{
		cfg:    cfg,
		logger: logger,
		handler: inbound.NewHandler(inbound.Config{
			EventLogPath:  cfg.EventLogPath,
			AutoReplyText: cfg.AutoReplyText,
		}, logger),
		agent:   agent.NewRunner(cfg.Agent, logger),
		desktop: desktop.DefaultQueue(),
		worker:  desktop.NewWorker(desktop.DefaultQueue(), logger, desktop.WorkerConfig{}),
	}
}

// Serve starts the long-connection client and waits until the context is canceled
// or the SDK returns an error.
func (s *Service) Serve(ctx context.Context) error {
	if s.agent != nil {
		defer func() {
			if err := s.agent.Close(); err != nil {
				s.logger.Printf("failed to close agent bridge: %v", err)
			}
		}()
	}

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
	dispatcher.OnP2MessageReadV1(s.handleMessageRead)

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

	if s.cfg.DesktopWorker {
		go func() {
			if err := s.worker.Serve(ctx); err != nil {
				s.logger.Printf("desktop worker stopped with error: %v", err)
			}
		}()
	}

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
	_ = ctx

	entry, err := buildLoggedEvent(event)
	if err != nil {
		return err
	}
	if err := s.handler.Process(entry); err != nil {
		return err
	}

	if request, ok := desktop.ExtractRequest(entry.MessageText); ok {
		task, err := s.desktop.Enqueue(entry, request)
		if err != nil {
			return err
		}
		ack := fmt.Sprintf("桌面 GUI 任务已加入队列，任务号 %s。我会在后台尝试执行，完成后回这里。", task.ID)
		if err := s.desktop.Reply(task, ack); err != nil {
			s.logger.Printf("failed to acknowledge desktop task %s: %v", task.ID, err)
		}
		return nil
	}

	s.agent.Dispatch(entry)
	return nil
}

func (s *Service) handleMessageRead(ctx context.Context, event *larkim.P2MessageReadV1) error {
	_ = ctx
	_ = event
	return nil
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
			input.ThreadID = stringValue(event.Event.Message.ThreadId)
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

// DefaultAgentConfig builds the Codex agent config from environment and config files.
func DefaultAgentConfig() agent.Config {
	timeoutMinutes := config.GetAgentTimeoutMinutes()
	if timeoutMinutes <= 0 {
		timeoutMinutes = 20
	}
	return agent.Config{
		Enabled:         config.GetAgentEnabled(),
		Backend:         config.GetAgentBackend(),
		CodexBinary:     config.GetAgentCodexBinary(),
		Workspace:       config.GetAgentWorkspace(),
		Model:           config.GetAgentModel(),
		ReasoningEffort: config.GetAgentReasoningEffort(),
		ThreadBindings:  config.GetAgentThreadBindingsPath(),
		AckText:         config.GetAgentAckText(),
		ResultMaxChars:  config.GetAgentResultMaxChars(),
		Timeout:         time.Duration(timeoutMinutes) * time.Minute,
	}
}
