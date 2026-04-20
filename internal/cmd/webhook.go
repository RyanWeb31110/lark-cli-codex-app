package cmd

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/yjwong/lark-cli/internal/config"
	"github.com/yjwong/lark-cli/internal/output"
	"github.com/yjwong/lark-cli/internal/webhook"
)

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Webhook and event subscription commands",
	Long:  "Run a local Feishu/Lark webhook server for event subscription callbacks.",
}

var (
	webhookListenAddr        string
	webhookPath              string
	webhookVerificationToken string
	webhookEventLogPath      string
	webhookAutoReplyText     string
)

var webhookServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the webhook callback server",
	Long: `Run a local webhook server for Feishu/Lark event subscriptions.

The current implementation supports plaintext event callbacks and URL verification.
Leave the Encrypt Key blank in the Feishu/Lark app console for this version.

Examples:
  lark webhook serve
  lark webhook serve --listen 0.0.0.0:8080 --path /webhook/feishu
  lark webhook serve --token my-verification-token
  lark webhook serve --auto-reply-text "收到：{{text}}"`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := webhook.Config{
			ListenAddr:        webhookListenAddr,
			Path:              webhookPath,
			VerificationToken: webhookVerificationToken,
			EventLogPath:      webhookEventLogPath,
			AutoReplyText:     webhookAutoReplyText,
		}
		if cfg.ListenAddr == "" {
			cfg.ListenAddr = config.GetWebhookListenAddr()
		}
		if cfg.Path == "" {
			cfg.Path = config.GetWebhookPath()
		}
		if cfg.VerificationToken == "" {
			cfg.VerificationToken = config.GetWebhookVerificationToken()
		}
		if cfg.EventLogPath == "" {
			cfg.EventLogPath = config.GetWebhookEventLogPath()
		}
		if cfg.AutoReplyText == "" {
			cfg.AutoReplyText = config.GetWebhookAutoReplyText()
		}

		server := webhook.New(cfg)
		output.JSON(map[string]interface{}{
			"ok":                   true,
			"listen":               cfg.ListenAddr,
			"path":                 cfg.Path,
			"event_log":            cfg.EventLogPath,
			"verification_token":   cfg.VerificationToken != "",
			"auto_reply_enabled":   cfg.AutoReplyText != "",
			"encryption_supported": false,
		})

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		if err := server.Serve(ctx); err != nil {
			output.Fatal("WEBHOOK_ERROR", err)
		}
	},
}

func init() {
	webhookServeCmd.Flags().StringVar(&webhookListenAddr, "listen", "", "listen address, e.g. 0.0.0.0:8080")
	webhookServeCmd.Flags().StringVar(&webhookPath, "path", "", "callback path")
	webhookServeCmd.Flags().StringVar(&webhookVerificationToken, "token", "", "verification token configured in Feishu/Lark event subscriptions")
	webhookServeCmd.Flags().StringVar(&webhookEventLogPath, "event-log", "", "path to JSONL event log file")
	webhookServeCmd.Flags().StringVar(&webhookAutoReplyText, "auto-reply-text", "", "optional plain-text auto-reply template; supports {{text}}, {{chat_id}}, {{message_id}}, {{sender_open_id}}")

	webhookCmd.AddCommand(webhookServeCmd)
}
