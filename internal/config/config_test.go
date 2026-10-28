package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantURL   string
		wantIndex string
		wantErr   string
	}{
		{
			name:      "defaults when unset",
			wantURL:   "http://localhost:9200",
			wantIndex: "motorcycles",
		},
		{
			name:      "values are trimmed",
			env:       map[string]string{"ELASTICSEARCH_URL": " http://127.0.0.1:9200 ", "ELASTICSEARCH_INDEX": " motorcycles-v2 "},
			wantURL:   "http://127.0.0.1:9200",
			wantIndex: "motorcycles-v2",
		},
		{name: "empty url", env: map[string]string{"ELASTICSEARCH_URL": ""}, wantErr: "invalid ELASTICSEARCH_URL: must not be empty"},
		{name: "url without scheme", env: map[string]string{"ELASTICSEARCH_URL": "localhost:9200"}, wantErr: "must use http or https scheme"},
		{name: "url without host", env: map[string]string{"ELASTICSEARCH_URL": "http://"}, wantErr: "must include a host"},
		{name: "empty index", env: map[string]string{"ELASTICSEARCH_INDEX": "  "}, wantErr: "invalid ELASTICSEARCH_INDEX: must not be empty"},
		{name: "uppercase index", env: map[string]string{"ELASTICSEARCH_INDEX": "Honda_Motorcycles"}, wantErr: "must be lowercase"},
		{name: "index starts with underscore", env: map[string]string{"ELASTICSEARCH_INDEX": "_motorcycles"}, wantErr: "must not start with"},
		{name: "index starts with dash", env: map[string]string{"ELASTICSEARCH_INDEX": "-motorcycles"}, wantErr: "must not start with"},
		{name: "index with space", env: map[string]string{"ELASTICSEARCH_INDEX": "honda motorcycles"}, wantErr: "must not contain"},
		{name: "index with wildcard", env: map[string]string{"ELASTICSEARCH_INDEX": "honda*"}, wantErr: "must not contain"},
		{name: "index with colon", env: map[string]string{"ELASTICSEARCH_INDEX": "cluster:honda"}, wantErr: "must not contain"},
		{name: "dot index", env: map[string]string{"ELASTICSEARCH_INDEX": "."}, wantErr: "is not allowed"},
		{name: "index too long", env: map[string]string{"ELASTICSEARCH_INDEX": strings.Repeat("a", 256)}, wantErr: "must not be longer than 255 bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnv(t, "ELASTICSEARCH_URL")
			unsetEnv(t, "ELASTICSEARCH_INDEX")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := Load()

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg.ElasticsearchURL != tt.wantURL {
				t.Errorf("ElasticsearchURL = %q, want %q", cfg.ElasticsearchURL, tt.wantURL)
			}
			if cfg.ElasticsearchIndex != tt.wantIndex {
				t.Errorf("ElasticsearchIndex = %q, want %q", cfg.ElasticsearchIndex, tt.wantIndex)
			}
		})
	}
}

// unsetEnv removes key for the duration of the test. t.Setenv registers the
// cleanup that restores the original value afterwards.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}
