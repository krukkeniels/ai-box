package main

import (
	"os"
	"strconv"
)

// Config holds all configuration for the LLM sidecar proxy.
// All values are read from environment variables with sensible defaults.
type Config struct {
	ListenAddr     string // AIBOX_LLM_PROXY_ADDR, default ":8443"
	Upstream       string // AIBOX_LLM_UPSTREAM, default "https://foundry.internal"
	SecretPath     string // AIBOX_LLM_SECRET_PATH, default "/run/secrets/llm-api-key"
	LogPath        string // AIBOX_LLM_LOG_PATH, default "/var/log/aibox/llm-payloads.jsonl"
	RateLimitRPM   int    // AIBOX_LLM_RATE_LIMIT_RPM, default 60
	RateLimitTPM   int    // AIBOX_LLM_RATE_LIMIT_TPM, default 100000
	MaxRequestSize int64  // AIBOX_LLM_MAX_REQUEST_SIZE, default 1048576 (1MB)
	SandboxID      string // AIBOX_SANDBOX_ID
	User           string // AIBOX_USER
}

// LoadConfig reads configuration from environment variables, applying defaults
// where values are not set.
func LoadConfig() Config {
	return Config{
		ListenAddr:     envOrDefault("AIBOX_LLM_PROXY_ADDR", ":8443"),
		Upstream:       envOrDefault("AIBOX_LLM_UPSTREAM", "https://foundry.internal"),
		SecretPath:     envOrDefault("AIBOX_LLM_SECRET_PATH", "/run/secrets/llm-api-key"),
		LogPath:        envOrDefault("AIBOX_LLM_LOG_PATH", "/var/log/aibox/llm-payloads.jsonl"),
		RateLimitRPM:   envOrDefaultInt("AIBOX_LLM_RATE_LIMIT_RPM", 60),
		RateLimitTPM:   envOrDefaultInt("AIBOX_LLM_RATE_LIMIT_TPM", 100000),
		MaxRequestSize: envOrDefaultInt64("AIBOX_LLM_MAX_REQUEST_SIZE", 1048576),
		SandboxID:      os.Getenv("AIBOX_SANDBOX_ID"),
		User:           os.Getenv("AIBOX_USER"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envOrDefaultInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}
