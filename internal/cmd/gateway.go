package cmd

import (
	"context"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/yjwong/lark-cli/internal/config"
	"github.com/yjwong/lark-cli/internal/gateway"
	"github.com/yjwong/lark-cli/internal/output"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Local gateway commands",
	Long:  "Run a local Feishu/Lark gateway using WebSocket long connections.",
}

var (
	gatewayEventLogPath         string
	gatewayAutoReplyText        string
	gatewayAgentEnabled         bool
	gatewayAgentBackend         string
	gatewayAgentWorkspace       string
	gatewayAgentReasoningEffort string
	gatewayAgentThreadBindings  string
	gatewayDesktopWorker        bool
)

var gatewayServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the local WebSocket gateway",
	Long: `Run a local Feishu/Lark gateway using WebSocket event subscriptions.

This mode is similar to OpenClaw's default channel transport:
- no public HTTPS callback URL is required
- the local process maintains an outbound WebSocket connection
- incoming bot messages are appended to a JSONL log

Examples:
  lark gateway serve
  lark gateway serve --auto-reply-text "收到：{{text}}"
  lark gateway serve --agent --agent-workspace ~/WorkSpace
  lark gateway serve --agent --agent-backend app_server --agent-reasoning-effort medium
  lark gateway serve --event-log ~/.lark/gateway-events.jsonl`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := gateway.Config{
			EventLogPath:  gatewayEventLogPath,
			AutoReplyText: gatewayAutoReplyText,
			Agent:         gateway.DefaultAgentConfig(),
			DesktopWorker: gatewayDesktopWorker,
		}
		if cfg.EventLogPath == "" {
			cfg.EventLogPath = config.GetGatewayEventLogPath()
		}
		if cfg.AutoReplyText == "" {
			cfg.AutoReplyText = config.GetGatewayAutoReplyText()
		}
		if cmd.Flags().Changed("agent") {
			cfg.Agent.Enabled = gatewayAgentEnabled
		}
		if strings.TrimSpace(gatewayAgentBackend) != "" {
			cfg.Agent.Backend = strings.TrimSpace(gatewayAgentBackend)
		}
		if strings.TrimSpace(gatewayAgentWorkspace) != "" {
			cfg.Agent.Workspace = strings.TrimSpace(gatewayAgentWorkspace)
		}
		if strings.TrimSpace(gatewayAgentReasoningEffort) != "" {
			cfg.Agent.ReasoningEffort = strings.TrimSpace(gatewayAgentReasoningEffort)
		}
		if strings.TrimSpace(gatewayAgentThreadBindings) != "" {
			cfg.Agent.ThreadBindings = strings.TrimSpace(gatewayAgentThreadBindings)
		}

		service := gateway.New(cfg)
		output.JSON(map[string]interface{}{
			"ok":                     true,
			"mode":                   "websocket",
			"region":                 config.GetRegion(),
			"event_log":              cfg.EventLogPath,
			"auto_reply_enabled":     cfg.AutoReplyText != "",
			"agent_enabled":          cfg.Agent.Enabled,
			"agent_backend":          cfg.Agent.Backend,
			"agent_workspace":        cfg.Agent.Workspace,
			"agent_reasoning_effort": cfg.Agent.ReasoningEffort,
			"agent_thread_bindings":  cfg.Agent.ThreadBindings,
			"desktop_worker":         cfg.DesktopWorker,
			"public_https_required":  false,
		})

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		if err := service.Serve(ctx); err != nil {
			output.Fatal("GATEWAY_ERROR", err)
		}
	},
}

func init() {
	gatewayServeCmd.Flags().StringVar(&gatewayEventLogPath, "event-log", "", "path to JSONL event log file")
	gatewayServeCmd.Flags().StringVar(&gatewayAutoReplyText, "auto-reply-text", "", "optional plain-text auto-reply template; supports {{text}}, {{chat_id}}, {{message_id}}, {{sender_open_id}}")
	gatewayServeCmd.Flags().BoolVar(&gatewayAgentEnabled, "agent", false, "dispatch inbound Feishu messages to the local Codex agent bridge")
	gatewayServeCmd.Flags().StringVar(&gatewayAgentBackend, "agent-backend", "", "Codex agent backend: app_server or codex_exec")
	gatewayServeCmd.Flags().StringVar(&gatewayAgentWorkspace, "agent-workspace", "", "workspace root used when the local Codex agent executes tasks")
	gatewayServeCmd.Flags().StringVar(&gatewayAgentReasoningEffort, "agent-reasoning-effort", "", "reasoning effort override for Codex tasks: minimal, low, medium, high, or xhigh")
	gatewayServeCmd.Flags().StringVar(&gatewayAgentThreadBindings, "agent-thread-bindings", "", "path to Lark chat/thread -> Codex thread binding JSON")
	gatewayServeCmd.Flags().BoolVar(&gatewayDesktopWorker, "desktop-worker", false, "run the local desktop task worker inside the gateway process")

	gatewayCmd.AddCommand(gatewayServeCmd)
}
