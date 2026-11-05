// Package config loads twelve-factor settings from the environment and the
// pilot's ward/ladder definition from YAML.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Mode selects demo (seeded fake data, simulated delivery, time controls) or
// pilot (real channels; simulator disabled).
type Mode string

// Modes.
const (
	ModeDemo  Mode = "demo"
	ModePilot Mode = "pilot"
)

// Config is the full runtime configuration.
type Config struct {
	Mode        Mode
	Addr        string
	BaseURL     string
	DatabaseURL string
	SecretKey   []byte
	RuleFile    string
	LadderFile  string
	LogLevel    string

	AdminUser     string
	AdminPassword string
	JudgeUser     string
	JudgePassword string

	PartnerName           string
	MissedCallNumber      string // shown on the flyer and public pages
	WhatsAppNumber        string
	AdvisoryReviewedBy    string
	RequirePartnerSignoff bool
	AudioRetention        time.Duration
	ConsentWindow         time.Duration
	FollowupWindow        time.Duration

	WhatsApp WhatsAppConfig
	Twilio   TwilioConfig
	Exotel   ExotelConfig
	Sarvam   SarvamConfig
	LLM      LLMConfig
	SMTP     SMTPConfig

	AlertInterval  time.Duration
	TimerInterval  time.Duration
	OutboxInterval time.Duration
}

// WhatsAppConfig configures the Meta WhatsApp Cloud API.
type WhatsAppConfig struct {
	Token         string
	PhoneNumberID string
	AppSecret     string
	VerifyToken   string
	APIVersion    string
	// Template names for business-initiated messages outside the 24h window.
	AdvisoryTemplate string
	CallbackTemplate string
}

// Enabled reports whether outbound WhatsApp is configured.
func (w WhatsAppConfig) Enabled() bool { return w.Token != "" && w.PhoneNumberID != "" }

// TwilioConfig configures Twilio Programmable Voice.
type TwilioConfig struct {
	AccountSID string
	AuthToken  string
	FromNumber string
}

// Enabled reports whether Twilio voice is configured.
func (t TwilioConfig) Enabled() bool {
	return t.AccountSID != "" && t.AuthToken != "" && t.FromNumber != ""
}

// ExotelConfig configures Exotel (Indian telephony).
type ExotelConfig struct {
	SID        string
	APIKey     string
	APIToken   string
	Subdomain  string
	CallerID   string
	AppID      string // Exotel flow (applet) id that hits our passthru URL
	WebhookKey string // shared secret expected as ?key= on Exotel callbacks
}

// Enabled reports whether Exotel is configured.
func (e ExotelConfig) Enabled() bool { return e.SID != "" && e.APIKey != "" && e.APIToken != "" }

// SarvamConfig configures Sarvam AI speech-to-text.
type SarvamConfig struct {
	APIKey  string
	Model   string
	BaseURL string
}

// LLMConfig configures the optional extraction/writing model.
type LLMConfig struct {
	Provider string // anthropic | openai (any OpenAI-compatible endpoint)
	APIKey   string
	Model    string
	BaseURL  string
}

// Enabled reports whether an LLM is configured.
func (l LLMConfig) Enabled() bool { return l.Provider != "" && l.APIKey != "" }

// SMTPConfig configures email delivery to officials.
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// Enabled reports whether email is configured.
func (s SMTPConfig) Enabled() bool { return s.Host != "" && s.From != "" }

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	c := &Config{
		Mode:        Mode(env("BHAGIRATH_MODE", "demo")),
		Addr:        env("BHAGIRATH_ADDR", ":"+env("PORT", "8080")),
		BaseURL:     strings.TrimRight(env("BHAGIRATH_BASE_URL", "http://localhost:8080"), "/"),
		DatabaseURL: env("DATABASE_URL", "postgres://bhagirath:bhagirath@localhost:5432/bhagirath?sslmode=disable"),
		RuleFile:    env("BHAGIRATH_RULE_FILE", "config/alert_rule_v1.yaml"),
		LadderFile:  env("BHAGIRATH_LADDER_FILE", "config/ladder.example.yaml"),
		LogLevel:    env("BHAGIRATH_LOG_LEVEL", "info"),

		AdminUser:     env("BHAGIRATH_ADMIN_USER", "admin"),
		AdminPassword: os.Getenv("BHAGIRATH_ADMIN_PASSWORD"),
		JudgeUser:     env("BHAGIRATH_JUDGE_USER", "judge"),
		JudgePassword: os.Getenv("BHAGIRATH_JUDGE_PASSWORD"),

		PartnerName:           env("BHAGIRATH_PARTNER_NAME", "the pilot partner"),
		MissedCallNumber:      os.Getenv("BHAGIRATH_MISSED_CALL_NUMBER"),
		WhatsAppNumber:        os.Getenv("BHAGIRATH_WHATSAPP_NUMBER"),
		AdvisoryReviewedBy:    os.Getenv("BHAGIRATH_ADVISORY_REVIEWED_BY"),
		RequirePartnerSignoff: envBool("BHAGIRATH_REQUIRE_PARTNER_SIGNOFF", true),
		AudioRetention:        time.Duration(envInt("BHAGIRATH_AUDIO_RETENTION_DAYS", 90)) * 24 * time.Hour,
		ConsentWindow:         24 * time.Hour,
		FollowupWindow:        24 * time.Hour,

		WhatsApp: WhatsAppConfig{
			Token:            os.Getenv("WHATSAPP_TOKEN"),
			PhoneNumberID:    os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
			AppSecret:        os.Getenv("WHATSAPP_APP_SECRET"),
			VerifyToken:      os.Getenv("WHATSAPP_VERIFY_TOKEN"),
			APIVersion:       env("WHATSAPP_API_VERSION", "v21.0"),
			AdvisoryTemplate: os.Getenv("WHATSAPP_ADVISORY_TEMPLATE"),
			CallbackTemplate: os.Getenv("WHATSAPP_CALLBACK_TEMPLATE"),
		},
		Twilio: TwilioConfig{
			AccountSID: os.Getenv("TWILIO_ACCOUNT_SID"),
			AuthToken:  os.Getenv("TWILIO_AUTH_TOKEN"),
			FromNumber: os.Getenv("TWILIO_FROM_NUMBER"),
		},
		Exotel: ExotelConfig{
			SID:        os.Getenv("EXOTEL_SID"),
			APIKey:     os.Getenv("EXOTEL_API_KEY"),
			APIToken:   os.Getenv("EXOTEL_API_TOKEN"),
			Subdomain:  env("EXOTEL_SUBDOMAIN", "api.exotel.com"),
			CallerID:   os.Getenv("EXOTEL_CALLER_ID"),
			AppID:      os.Getenv("EXOTEL_APP_ID"),
			WebhookKey: os.Getenv("EXOTEL_WEBHOOK_KEY"),
		},
		Sarvam: SarvamConfig{
			APIKey:  os.Getenv("SARVAM_API_KEY"),
			Model:   env("SARVAM_STT_MODEL", "saarika:v2.5"),
			BaseURL: env("SARVAM_BASE_URL", "https://api.sarvam.ai"),
		},
		LLM: LLMConfig{
			Provider: os.Getenv("LLM_PROVIDER"),
			APIKey:   os.Getenv("LLM_API_KEY"),
			Model:    os.Getenv("LLM_MODEL"),
			BaseURL:  os.Getenv("LLM_BASE_URL"),
		},
		SMTP: SMTPConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     envInt("SMTP_PORT", 587),
			User:     os.Getenv("SMTP_USER"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     os.Getenv("SMTP_FROM"),
		},
		AlertInterval:  envDuration("BHAGIRATH_ALERT_INTERVAL", 15*time.Minute),
		TimerInterval:  envDuration("BHAGIRATH_TIMER_INTERVAL", 5*time.Minute),
		OutboxInterval: envDuration("BHAGIRATH_OUTBOX_INTERVAL", 15*time.Second),
	}
	if c.Mode != ModeDemo && c.Mode != ModePilot {
		return nil, fmt.Errorf("BHAGIRATH_MODE must be demo or pilot, got %q", c.Mode)
	}
	key := os.Getenv("BHAGIRATH_SECRET_KEY")
	switch {
	case len(key) >= 32:
		c.SecretKey = []byte(key)
	case key == "" && c.Mode == ModeDemo:
		// Demo deployments hold only fake data; a fixed key keeps links stable
		// across restarts. Pilot mode refuses to start without a real key.
		c.SecretKey = []byte("bhagirath-demo-only-secret-key-do-not-use-in-pilot")
	default:
		return nil, errors.New("BHAGIRATH_SECRET_KEY must be set to at least 32 random bytes in pilot mode")
	}
	if c.Mode == ModePilot {
		if c.AdminPassword == "" || len(c.AdminPassword) < 12 {
			return nil, errors.New("BHAGIRATH_ADMIN_PASSWORD (12+ chars) is required in pilot mode")
		}
		if c.LadderFile == "config/ladder.example.yaml" {
			return nil, errors.New("pilot mode needs a real BHAGIRATH_LADDER_FILE (the example contains demo officials only)")
		}
	}
	return c, nil
}

// Demo reports whether this is a demo deployment.
func (c *Config) Demo() bool { return c.Mode == ModeDemo }

// Providers summarises which real channels are live (for the dashboard).
func (c *Config) Providers() map[string]bool {
	return map[string]bool{
		"whatsapp": c.WhatsApp.Enabled(),
		"twilio":   c.Twilio.Enabled(),
		"exotel":   c.Exotel.Enabled(),
		"sarvam":   c.Sarvam.APIKey != "",
		"llm":      c.LLM.Enabled(),
		"email":    c.SMTP.Enabled(),
	}
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	if v, err := strconv.ParseBool(os.Getenv(k)); err == nil {
		return v
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}
