package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	yaml := `
listen: ":9090"
defaults:
  timeout: 3s
  max_results: 5
providers:
  - name: tavily
    tier: 1
    api_key: test-key
    rate_limit:
      requests: 1000
      window: 720h
    timeout: 2s
  - name: searxng
    tier: 2
    api_base: http://searxng:8888
    timeout: 5s
routing:
  strategy: round-robin
circuit_breaker:
  threshold: 5
  timeout: 60s
metrics:
  enabled: true
  path: /metrics
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Listen != ":9090" {
		t.Errorf("Listen = %q, want %q", cfg.Listen, ":9090")
	}
	if cfg.Defaults.Timeout != 3*time.Second {
		t.Errorf("Defaults.Timeout = %v, want %v", cfg.Defaults.Timeout, 3*time.Second)
	}
	if cfg.Defaults.MaxResults != 5 {
		t.Errorf("Defaults.MaxResults = %d, want 5", cfg.Defaults.MaxResults)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("len(Providers) = %d, want 2", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "tavily" {
		t.Errorf("Providers[0].Name = %q, want %q", cfg.Providers[0].Name, "tavily")
	}
	if cfg.Providers[0].Tier != 1 {
		t.Errorf("Providers[0].Tier = %d, want 1", cfg.Providers[0].Tier)
	}
	if cfg.Providers[0].APIKey != "test-key" {
		t.Errorf("Providers[0].APIKey = %q, want %q", cfg.Providers[0].APIKey, "test-key")
	}
	if cfg.Providers[0].RateLimit.Requests != 1000 {
		t.Errorf("RateLimit.Requests = %d, want 1000", cfg.Providers[0].RateLimit.Requests)
	}
	if cfg.Providers[0].RateLimit.Window != 720*time.Hour {
		t.Errorf("RateLimit.Window = %v, want 720h", cfg.Providers[0].RateLimit.Window)
	}
	if cfg.Routing.Strategy != "round-robin" {
		t.Errorf("Strategy = %q, want %q", cfg.Routing.Strategy, "round-robin")
	}
	if cfg.CircuitBreaker.Threshold != 5 {
		t.Errorf("CB Threshold = %d, want 5", cfg.CircuitBreaker.Threshold)
	}
}

func TestLoadConfigEnvSubstitution(t *testing.T) {
	t.Setenv("TEST_TAVILY_KEY", "tvly-secret-123")

	yaml := `
listen: ":8080"
providers:
  - name: tavily
    tier: 1
    api_key: ${TEST_TAVILY_KEY}
    timeout: 3s
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Providers[0].APIKey != "tvly-secret-123" {
		t.Errorf("APIKey = %q, want %q", cfg.Providers[0].APIKey, "tvly-secret-123")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	yaml := `
providers:
  - name: searxng
    tier: 2
    api_base: http://searxng:8888
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Listen != ":8080" {
		t.Errorf("Listen = %q, want default %q", cfg.Listen, ":8080")
	}
	if cfg.Defaults.Timeout != 5*time.Second {
		t.Errorf("Defaults.Timeout = %v, want default %v", cfg.Defaults.Timeout, 5*time.Second)
	}
	if cfg.Defaults.MaxResults != 10 {
		t.Errorf("Defaults.MaxResults = %d, want default 10", cfg.Defaults.MaxResults)
	}
	if cfg.Routing.Strategy != "round-robin" {
		t.Errorf("Strategy = %q, want default %q", cfg.Routing.Strategy, "round-robin")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "no providers",
			cfg:     Config{},
			wantErr: true,
		},
		{
			name: "provider missing name",
			cfg: Config{
				Providers: []ProviderConfig{{Tier: 1}},
			},
			wantErr: true,
		},
		{
			name: "provider missing tier",
			cfg: Config{
				Providers: []ProviderConfig{{Name: "tavily"}},
			},
			wantErr: true,
		},
		{
			name: "valid minimal config",
			cfg: Config{
				Providers: []ProviderConfig{{Name: "searxng", Tier: 1, APIBase: "http://localhost:8888"}},
			},
			wantErr: false,
		},
		{
			name: "multiple providers valid",
			cfg: Config{
				Providers: []ProviderConfig{
					{Name: "tavily", Tier: 1, APIKey: "key1"},
					{Name: "brave", Tier: 1, APIKey: "key2"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
