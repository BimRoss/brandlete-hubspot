package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Port           string
	HubSpotToken   string
	WebhookSecret  string
	MCPBearerToken string
	DryRun         bool
}

func Load() (Config, error) {
	cfg := Config{
		Port:           env("PORT", "8080"),
		HubSpotToken:   os.Getenv("HUBSPOT_TOKEN"),
		WebhookSecret:  os.Getenv("WEBHOOK_SECRET"),
		MCPBearerToken: os.Getenv("MCP_BEARER_TOKEN"),
	}
	dryRunStr := env("DRY_RUN", "false")
	dr, err := strconv.ParseBool(dryRunStr)
	if err != nil {
		return cfg, errors.New("DRY_RUN must be a boolean")
	}
	cfg.DryRun = dr

	if !cfg.DryRun && cfg.HubSpotToken == "" {
		return cfg, errors.New("HUBSPOT_TOKEN is required when DRY_RUN is false")
	}
	if cfg.WebhookSecret == "" {
		return cfg, errors.New("WEBHOOK_SECRET is required")
	}
	if cfg.MCPBearerToken == "" {
		return cfg, errors.New("MCP_BEARER_TOKEN is required")
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
