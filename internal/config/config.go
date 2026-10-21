// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	defaultElasticsearchURL   = "http://localhost:9200"
	defaultElasticsearchIndex = "products"
)

type Config struct {
	ElasticsearchURL   string
	ElasticsearchIndex string
}

// Load reads configuration from the environment. Unset variables fall back to
// local development defaults; variables set to an empty value are rejected.
func Load() (Config, error) {
	cfg := Config{
		ElasticsearchURL:   envOrDefault("ELASTICSEARCH_URL", defaultElasticsearchURL),
		ElasticsearchIndex: envOrDefault("ELASTICSEARCH_INDEX", defaultElasticsearchIndex),
	}

	if err := validateElasticsearchURL(cfg.ElasticsearchURL); err != nil {
		return Config{}, fmt.Errorf("invalid ELASTICSEARCH_URL: %w", err)
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return strings.TrimSpace(value)
}

func validateElasticsearchURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("must not be empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%q must use http or https scheme", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("%q must include a host", raw)
	}

	return nil
}
