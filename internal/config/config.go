package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	DatabaseURL    string
	RedisURL       string
	ReaperInterval time.Duration
	SandboxTTL     time.Duration
	PausedTTL      time.Duration
	WarmPoolSize   int
	LogLevel       string
	MetricsToken   string
}

func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, relying on system env vars")
	}

	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
	}
	cfg.LogLevel = getEnv("LOG_LEVEL", "info")
	cfg.MetricsToken = getEnv("METRICS_TOKEN", "")

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	reaperInterval, err := time.ParseDuration(getEnv("REAPER_INTERVAL", "30s"))
	if err != nil {
		return nil, fmt.Errorf("invalid REAPER_INTERVAL: %w", err)
	}
	cfg.ReaperInterval = reaperInterval

	sandboxTTL, err := time.ParseDuration(getEnv("SANDBOX_TTL", "1h"))
	if err != nil {
		return nil, fmt.Errorf("invalid SANDBOX_TTL: %w", err)
	}
	cfg.SandboxTTL = sandboxTTL

	warmPoolSize, err := strconv.Atoi(getEnv("WARM_POOL_SIZE", "2"))
	if err != nil {
		return nil, fmt.Errorf("invalid WARM_POOL_SIZE: %w", err)
	}
	cfg.WarmPoolSize = warmPoolSize

	pausedTTL, err := time.ParseDuration(getEnv("PAUSED_SANDBOX_TTL", "24h"))
	if err != nil {
		return nil, fmt.Errorf("invalid PAUSED_SANDBOX_TTL: %w", err)
	}
	cfg.PausedTTL = pausedTTL

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
