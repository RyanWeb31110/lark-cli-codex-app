package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	AppID     string `mapstructure:"app_id"`
	AppSecret string `mapstructure:"app_secret"`
	Region    string `mapstructure:"region"`
	Defaults  struct {
		Timezone        string `mapstructure:"timezone"`
		ReminderMinutes int    `mapstructure:"reminder_minutes"`
	} `mapstructure:"defaults"`
	OAuth struct {
		RedirectPort int `mapstructure:"redirect_port"`
	} `mapstructure:"oauth"`
	Gateway struct {
		EventLog      string `mapstructure:"event_log"`
		AutoReplyText string `mapstructure:"auto_reply_text"`
	} `mapstructure:"gateway"`
	Webhook struct {
		ListenAddr        string `mapstructure:"listen_addr"`
		Path              string `mapstructure:"path"`
		VerificationToken string `mapstructure:"verification_token"`
		EventLog          string `mapstructure:"event_log"`
		AutoReplyText     string `mapstructure:"auto_reply_text"`
	} `mapstructure:"webhook"`
	CustomEmojis map[string]string `mapstructure:"custom_emojis"`
}

var (
	cfg     *Config
	cfgDir  string
	rootDir string
)

// GetConfigDir returns the .lark directory path
func GetConfigDir() string {
	return cfgDir
}

// GetRootDir returns the project root directory
func GetRootDir() string {
	return rootDir
}

// Init initializes the configuration
func Init() error {
	// Config directory can be set via LARK_CONFIG_DIR or legacy LARK_CAL_CONFIG_DIR
	cfgDir = os.Getenv("LARK_CONFIG_DIR")
	if cfgDir == "" {
		cfgDir = os.Getenv("LARK_CAL_CONFIG_DIR") // Legacy fallback
	}
	if cfgDir == "" {
		return fmt.Errorf("LARK_CONFIG_DIR environment variable is not set")
	}

	rootDir = filepath.Dir(cfgDir)

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(cfgDir)

	// Set defaults
	viper.SetDefault("region", "lark")
	viper.SetDefault("defaults.timezone", "Asia/Singapore")
	viper.SetDefault("defaults.reminder_minutes", 15)
	viper.SetDefault("oauth.redirect_port", 9999)
	viper.SetDefault("gateway.event_log", filepath.Join(cfgDir, "gateway-events.jsonl"))
	viper.SetDefault("webhook.listen_addr", "0.0.0.0:8080")
	viper.SetDefault("webhook.path", "/webhook/feishu")
	viper.SetDefault("webhook.event_log", filepath.Join(cfgDir, "webhook-events.jsonl"))

	// Environment variable bindings
	viper.SetEnvPrefix("LARK")
	viper.BindEnv("app_id", "LARK_APP_ID")
	viper.BindEnv("app_secret", "LARK_APP_SECRET")
	viper.BindEnv("gateway.event_log", "LARK_GATEWAY_EVENT_LOG")
	viper.BindEnv("gateway.auto_reply_text", "LARK_GATEWAY_AUTO_REPLY_TEXT")
	viper.BindEnv("webhook.listen_addr", "LARK_WEBHOOK_LISTEN")
	viper.BindEnv("webhook.path", "LARK_WEBHOOK_PATH")
	viper.BindEnv("webhook.verification_token", "LARK_WEBHOOK_TOKEN")
	viper.BindEnv("webhook.event_log", "LARK_WEBHOOK_EVENT_LOG")
	viper.BindEnv("webhook.auto_reply_text", "LARK_WEBHOOK_AUTO_REPLY_TEXT")

	// Read config file (if exists)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("error reading config: %w", err)
		}
		// Config file not found is OK, we'll use defaults and env vars
	}

	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

// Get returns the current configuration
func Get() *Config {
	if cfg == nil {
		cfg = &Config{}
	}
	return cfg
}

// GetAppID returns the app ID from config or environment
func GetAppID() string {
	return viper.GetString("app_id")
}

// GetAppSecret returns the app secret from environment
func GetAppSecret() string {
	return viper.GetString("app_secret")
}

// GetRegion returns the API/auth region: lark (default) or feishu
func GetRegion() string {
	region := strings.ToLower(strings.TrimSpace(viper.GetString("region")))
	switch region {
	case "feishu":
		return "feishu"
	case "lark", "":
		return "lark"
	default:
		return "lark"
	}
}

// GetTimezone returns the default timezone
func GetTimezone() string {
	return viper.GetString("defaults.timezone")
}

// GetRedirectPort returns the OAuth redirect port
func GetRedirectPort() int {
	return viper.GetInt("oauth.redirect_port")
}

// GetGatewayEventLogPath returns the JSONL path used for gateway event persistence.
func GetGatewayEventLogPath() string {
	path := strings.TrimSpace(viper.GetString("gateway.event_log"))
	if path == "" {
		return filepath.Join(cfgDir, "gateway-events.jsonl")
	}
	if !filepath.IsAbs(path) {
		return filepath.Join(rootDir, path)
	}
	return path
}

// GetGatewayAutoReplyText returns the optional static auto-reply text.
func GetGatewayAutoReplyText() string {
	return viper.GetString("gateway.auto_reply_text")
}

// GetWebhookListenAddr returns the listen address for webhook server.
func GetWebhookListenAddr() string {
	return viper.GetString("webhook.listen_addr")
}

// GetWebhookPath returns the webhook callback path.
func GetWebhookPath() string {
	path := strings.TrimSpace(viper.GetString("webhook.path"))
	if path == "" {
		return "/webhook/feishu"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

// GetWebhookVerificationToken returns the verification token for event callbacks.
func GetWebhookVerificationToken() string {
	return viper.GetString("webhook.verification_token")
}

// GetWebhookEventLogPath returns the JSONL path used for webhook event persistence.
func GetWebhookEventLogPath() string {
	path := strings.TrimSpace(viper.GetString("webhook.event_log"))
	if path == "" {
		return filepath.Join(cfgDir, "webhook-events.jsonl")
	}
	if !filepath.IsAbs(path) {
		return filepath.Join(rootDir, path)
	}
	return path
}

// GetWebhookAutoReplyText returns the optional static auto-reply text.
func GetWebhookAutoReplyText() string {
	return viper.GetString("webhook.auto_reply_text")
}

// TokensFilePath returns the path to the tokens file
func TokensFilePath() string {
	return filepath.Join(cfgDir, "tokens.json")
}

// TenantTokensFilePath returns the path to the tenant tokens file
func TenantTokensFilePath() string {
	return filepath.Join(cfgDir, "tenant_tokens.json")
}

// GetCustomEmojis returns the custom emoji mappings
func GetCustomEmojis() map[string]string {
	return viper.GetStringMapString("custom_emojis")
}
