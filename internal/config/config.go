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
	defaultElasticsearchIndex = "motorcycles"
	defaultHTTPAddr           = "127.0.0.1:8080"

	// Elasticsearch rejects index names longer than 255 bytes.
	maxIndexNameBytes = 255
)

type Config struct {
	ElasticsearchURL   string
	ElasticsearchIndex string
	HTTPAddr           string
}

// Load reads configuration from the environment. Unset variables fall back to
// local development defaults. Values that are set are trimmed and validated,
// so an empty or malformed value is rejected instead of silently replaced.
func Load() (Config, error) {
	cfg := Config{
		ElasticsearchURL:   envOrDefault("ELASTICSEARCH_URL", defaultElasticsearchURL),
		ElasticsearchIndex: envOrDefault("ELASTICSEARCH_INDEX", defaultElasticsearchIndex),
		HTTPAddr:           envOrDefault("HTTP_ADDR", defaultHTTPAddr),
	}

	if err := validateElasticsearchURL(cfg.ElasticsearchURL); err != nil {
		return Config{}, fmt.Errorf("invalid ELASTICSEARCH_URL: %w", err)
	}
	if err := validateIndexName(cfg.ElasticsearchIndex); err != nil {
		return Config{}, fmt.Errorf("invalid ELASTICSEARCH_INDEX: %w", err)
	}
	if cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("invalid HTTP_ADDR: must not be empty")
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

// validateIndexName checks the Elasticsearch index naming rules that a
// hand-written config value can realistically break. Failing here gives a
// clear startup error instead of an invalid_index_name_exception later.
func validateIndexName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("must not be empty")
	case name == "." || name == "..":
		return fmt.Errorf("%q is not allowed", name)
	case len(name) > maxIndexNameBytes:
		return fmt.Errorf("%q must not be longer than %d bytes", name, maxIndexNameBytes)
	case name != strings.ToLower(name):
		return fmt.Errorf("%q must be lowercase", name)
	case strings.ContainsAny(name[:1], "-_+"):
		return fmt.Errorf("%q must not start with '-', '_' or '+'", name)
	case strings.ContainsAny(name, `\/*?"<>| ,#:`):
		return fmt.Errorf(`%q must not contain \ / * ? " < > | space , # or :`, name)
	}

	return nil
}
