package config

import (
	"os"
	"time"
)

// Config holds all environment-driven settings for the dosedu.kz backend.
type Config struct {
	Env                   string
	HTTPPort              string
	PostgresDSN           string
	RedisAddr             string
	RedisPassword         string
	JWTSecret             string
	SessionTTL            time.Duration // idle-timeout for teacher/director/super-admin sessions
	TelegramBotToken      string
	TelegramWebhookSecret string // compared against X-Telegram-Bot-Api-Secret-Token
	MaxUploadBytes        int64  // e.g. 5 MB limit for level-test file uploads
	AnthropicAPIKey       string // optional — AI quiz-mistake explanations disabled until set
	MainSiteURL           string // e.g. https://dosedu.kz — self-checked by /s-admin/health
	AppSiteURL            string // e.g. https://app.dosedu.kz (student/parent portal) — self-checked by /s-admin/health
}

func Load() Config {
	return Config{
		Env:                   getEnv("APP_ENV", "development"),
		HTTPPort:              getEnv("HTTP_PORT", "8080"),
		PostgresDSN:           getEnv("POSTGRES_DSN", "postgres://dosedu:dosedu@localhost:5432/dosedu?sslmode=disable"),
		RedisAddr:             getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:         getEnv("REDIS_PASSWORD", ""),
		JWTSecret:             getEnv("JWT_SECRET", "CHANGE_ME_IN_PRODUCTION"),
		SessionTTL:            30 * time.Minute, // per spec: 30 min idle timeout for staff roles
		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramWebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		MaxUploadBytes:        5 * 1024 * 1024, // 5 MB, per teacher upload panel spec
		AnthropicAPIKey:       getEnv("ANTHROPIC_API_KEY", ""),
		MainSiteURL:           getEnv("MAIN_SITE_URL", ""),
		AppSiteURL:            getEnv("APP_SITE_URL", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
