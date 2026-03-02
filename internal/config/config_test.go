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
  timeout: 5s
  max_results: 20
  safe_search: 1
  language: "en"
providers:
  - name: tavily
    tier: primary
    api_key: "test-key-tavily"
    api_base: "https://api.tavily.com"
    rate_limit: 100
    timeout: 3s
  - name: brave
    tier: fallback
    api_key: "test-key-brave"
    api_base: "https://api.search.brave.com"
    rate_limit: 50
    timeout: 4s
routing:
  strategy: "tiered"
  max_fallbacks: 2
circuit_breaker:
  threshold: 5
  timeout: 30s
metrics:
  enabled: true
  path: "/metrics"
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	// Listen
	if cfg.Listen != ":9090" {
		t.Errorf("Listen = %q, want %q", cfg.Listen, ":9090")
	}

	// Defaults
	if cfg.Defaults.Timeout != 5*time.Second {
		t.Errorf("Defaults.Timeout = %v, want %v", cfg.Defaults.Timeout, 5*time.Second)
	}
	if cfg.Defaults.MaxResults != 20 {
		t.Errorf("Defaults.MaxResults = %d, want %d", cfg.Defaults.MaxResults, 20)
	}
	if cfg.Defaults.SafeSearch != 1 {
		t.Errorf("Defaults.SafeSearch = %d, want %d", cfg.Defaults.SafeSearch, 1)
	}
	if cfg.Defaults.Language != "en" {
		t.Errorf("Defaults.Language = %q, want %q", cfg.Defaults.Language, "en")
	}

	// Providers
	if len(cfg.Providers) != 2 {
		t.Fatalf("len(Providers) = %d, want 2", len(cfg.Providers))
	}
	p0 := cfg.Providers[0]
	if p0.Name != "tavily" {
		t.Errorf("Providers[0].Name = %q, want %q", p0.Name, "tavily")
	}
	if p0.Tier != "primary" {
		t.Errorf("Providers[0].Tier = %q, want %q", p0.Tier, "primary")
	}
	if p0.APIKey != "test-key-tavily" {
		t.Errorf("Providers[0].APIKey = %q, want %q", p0.APIKey, "test-key-tavily")
	}
	if p0.APIBase != "https://api.tavily.com" {
		t.Errorf("Providers[0].APIBase = %q, want %q", p0.APIBase, "https://api.tavily.com")
	}
	if p0.RateLimit != 100 {
		t.Errorf("Providers[0].RateLimit = %d, want %d", p0.RateLimit, 100)
	}
	if p0.Timeout != 3*time.Second {
		t.Errorf("Providers[0].Timeout = %v, want %v", p0.Timeout, 3*time.Second)
	}

	p1 := cfg.Providers[1]
	if p1.Name != "brave" {
		t.Errorf("Providers[1].Name = %q, want %q", p1.Name, "brave")
	}
	if p1.Tier != "fallback" {
		t.Errorf("Providers[1].Tier = %q, want %q", p1.Tier, "fallback")
	}
	if p1.RateLimit != 50 {
		t.Errorf("Providers[1].RateLimit = %d, want %d", p1.RateLimit, 50)
	}

	// Routing
	if cfg.Routing.Strategy != "tiered" {
		t.Errorf("Routing.Strategy = %q, want %q", cfg.Routing.Strategy, "tiered")
	}
	if cfg.Routing.MaxFallbacks != 2 {
		t.Errorf("Routing.MaxFallbacks = %d, want %d", cfg.Routing.MaxFallbacks, 2)
	}

	// CircuitBreaker
	if cfg.CircuitBreaker.Threshold != 5 {
		t.Errorf("CircuitBreaker.Threshold = %d, want %d", cfg.CircuitBreaker.Threshold, 5)
	}
	if cfg.CircuitBreaker.Timeout != 30*time.Second {
		t.Errorf("CircuitBreaker.Timeout = %v, want %v", cfg.CircuitBreaker.Timeout, 30*time.Second)
	}

	// Metrics
	if !cfg.Metrics.Enabled {
		t.Error("Metrics.Enabled = false, want true")
	}
	if cfg.Metrics.Path != "/metrics" {
		t.Errorf("Metrics.Path = %q, want %q", cfg.Metrics.Path, "/metrics")
	}
}

func TestLoadConfigEnvSubstitution(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "env-tavily-secret")
	t.Setenv("BRAVE_API_KEY", "env-brave-secret")
	t.Setenv("LISTEN_ADDR", ":7070")

	yaml := `
listen: "${LISTEN_ADDR}"
providers:
  - name: tavily
    tier: primary
    api_key: "${TAVILY_API_KEY}"
  - name: brave
    tier: fallback
    api_key: "${BRAVE_API_KEY}"
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Listen != ":7070" {
		t.Errorf("Listen = %q, want %q (env substitution failed)", cfg.Listen, ":7070")
	}
	if cfg.Providers[0].APIKey != "env-tavily-secret" {
		t.Errorf("Providers[0].APIKey = %q, want %q", cfg.Providers[0].APIKey, "env-tavily-secret")
	}
	if cfg.Providers[1].APIKey != "env-brave-secret" {
		t.Errorf("Providers[1].APIKey = %q, want %q", cfg.Providers[1].APIKey, "env-brave-secret")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	yaml := `
providers:
  - name: tavily
    tier: primary
    api_key: "key"
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	// Listen default
	if cfg.Listen != ":8080" {
		t.Errorf("Listen = %q, want default %q", cfg.Listen, ":8080")
	}

	// Defaults defaults
	if cfg.Defaults.Timeout != 10*time.Second {
		t.Errorf("Defaults.Timeout = %v, want default %v", cfg.Defaults.Timeout, 10*time.Second)
	}
	if cfg.Defaults.MaxResults != 10 {
		t.Errorf("Defaults.MaxResults = %d, want default %d", cfg.Defaults.MaxResults, 10)
	}
	if cfg.Defaults.SafeSearch != 0 {
		t.Errorf("Defaults.SafeSearch = %d, want default %d", cfg.Defaults.SafeSearch, 0)
	}
	if cfg.Defaults.Language != "" {
		t.Errorf("Defaults.Language = %q, want default %q", cfg.Defaults.Language, "")
	}

	// Routing defaults
	if cfg.Routing.Strategy != "tiered" {
		t.Errorf("Routing.Strategy = %q, want default %q", cfg.Routing.Strategy, "tiered")
	}
	if cfg.Routing.MaxFallbacks != 2 {
		t.Errorf("Routing.MaxFallbacks = %d, want default %d", cfg.Routing.MaxFallbacks, 2)
	}

	// CircuitBreaker defaults
	if cfg.CircuitBreaker.Threshold != 5 {
		t.Errorf("CircuitBreaker.Threshold = %d, want default %d", cfg.CircuitBreaker.Threshold, 5)
	}
	if cfg.CircuitBreaker.Timeout != 30*time.Second {
		t.Errorf("CircuitBreaker.Timeout = %v, want default %v", cfg.CircuitBreaker.Timeout, 30*time.Second)
	}

	// Metrics defaults
	if cfg.Metrics.Enabled {
		t.Error("Metrics.Enabled = true, want default false")
	}
	if cfg.Metrics.Path != "/metrics" {
		t.Errorf("Metrics.Path = %q, want default %q", cfg.Metrics.Path, "/metrics")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: Config{
				Providers: []ProviderConfig{
					{Name: "tavily", Tier: "primary", APIKey: "key"},
				},
			},
			wantErr: false,
		},
		{
			name: "no providers",
			cfg: Config{
				Providers: []ProviderConfig{},
			},
			wantErr: true,
			errMsg:  "at least one provider",
		},
		{
			name: "nil providers",
			cfg: Config{
				Providers: nil,
			},
			wantErr: true,
			errMsg:  "at least one provider",
		},
		{
			name: "provider missing name",
			cfg: Config{
				Providers: []ProviderConfig{
					{Name: "", Tier: "primary", APIKey: "key"},
				},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "provider missing tier",
			cfg: Config{
				Providers: []ProviderConfig{
					{Name: "tavily", Tier: "", APIKey: "key"},
				},
			},
			wantErr: true,
			errMsg:  "tier is required",
		},
		{
			name: "multiple providers valid",
			cfg: Config{
				Providers: []ProviderConfig{
					{Name: "tavily", Tier: "primary", APIKey: "key1"},
					{Name: "brave", Tier: "fallback", APIKey: "key2"},
				},
			},
			wantErr: false,
		},
		{
			name: "second provider invalid",
			cfg: Config{
				Providers: []ProviderConfig{
					{Name: "tavily", Tier: "primary", APIKey: "key1"},
					{Name: "", Tier: "fallback", APIKey: "key2"},
				},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Validate() returned nil, want error")
				}
				if tt.errMsg != "" {
					if got := err.Error(); !contains(got, tt.errMsg) {
						t.Errorf("error = %q, want it to contain %q", got, tt.errMsg)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("Validate() returned error: %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
