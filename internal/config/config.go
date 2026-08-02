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
	Port                 string
	DatabaseURL          string
	RedisURL             string
	ReaperInterval       time.Duration
	SandboxTTL           time.Duration
	PausedTTL            time.Duration
	WarmPoolSize         int
	LogLevel             string
	MetricsToken         string
	IsolationBackend     string // docker or firecracker
	FirecrackerBin       string
	FirecrackerKernel    string
	FirecrackerRootfsDir string
	FirecrackerRunDir    string
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
	cfg.IsolationBackend = getEnv("ISOLATION_BACKEND", "docker")
	cfg.FirecrackerBin = getEnv("FIRECRACKER_BIN", "/usr/local/bin/firecracker")
	cfg.FirecrackerKernel = getEnv("FIRECRACKER_KERNEL", "/var/lib/cage/vmlinux.bin")
	cfg.FirecrackerRootfsDir = getEnv("FIRECRACKER_ROOTFS_DIR", "/var/lib/cage/rootfs-base")
	cfg.FirecrackerRunDir = getEnv("FIRECRACKER_RUN_DIR", "/var/lib/cage/fc-run")

	cfg.IsolationBackend = getEnv("ISOLATION_BACKEND", "docker")
	if cfg.IsolationBackend != "docker" && cfg.IsolationBackend != "firecracker" {
		return nil, fmt.Errorf("ISOLATION_BACKEND must be 'docker' or 'firecracker',  got %q", cfg.IsolationBackend)
	}

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
