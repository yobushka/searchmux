package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for searchmux.
type Config struct {
	Listen         string               `yaml:"listen"`
	Defaults       DefaultsConfig       `yaml:"defaults"`
	Providers      []ProviderConfig     `yaml:"providers"`
	Routing        RoutingConfig        `yaml:"routing"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
	Metrics        MetricsConfig        `yaml:"metrics"`
	Auth           AuthConfig           `yaml:"auth"`
}

// AuthConfig stores only SHA-256 hashes of gateway bearer tokens.
type AuthConfig struct {
	TokenHashes []string `yaml:"token_hashes"`
}

// DefaultsConfig holds default search parameters.
type DefaultsConfig struct {
	Timeout    time.Duration `yaml:"timeout"`
	MaxResults int           `yaml:"max_results"`
	SafeSearch int           `yaml:"safe_search"`
	Language   string        `yaml:"language"`
}

// ProviderConfig holds configuration for a single search provider.
type ProviderConfig struct {
	Name      string        `yaml:"name"`
	Tier      int           `yaml:"tier"`
	APIBase   string        `yaml:"api_base"`
	RateLimit RateLimitConf `yaml:"rate_limit"`
	Timeout   time.Duration `yaml:"timeout"`
}

// RateLimitConf defines rate limit parameters for a provider.
type RateLimitConf struct {
	Requests int           `yaml:"requests"`
	Window   time.Duration `yaml:"window"`
}

// RoutingConfig controls how requests are routed across providers.
type RoutingConfig struct {
	Strategy string `yaml:"strategy"`
}

// CircuitBreakerConfig controls circuit breaker behavior.
type CircuitBreakerConfig struct {
	Threshold int           `yaml:"threshold"`
	Timeout   time.Duration `yaml:"timeout"`
	Window    time.Duration `yaml:"window"`
	Cooldown  time.Duration `yaml:"cooldown"`
}

// MetricsConfig controls Prometheus metrics exposure.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// substituteEnv replaces ${VAR} patterns with environment variable values.
func substituteEnv(data []byte) []byte {
	return envVarRe.ReplaceAllFunc(data, func(match []byte) []byte {
		varName := envVarRe.FindSubmatch(match)[1]
		if val, ok := os.LookupEnv(string(varName)); ok {
			return []byte(val)
		}
		return match
	})
}

// applyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.Defaults.Timeout == 0 {
		c.Defaults.Timeout = 5 * time.Second
	}
	if c.Defaults.MaxResults == 0 {
		c.Defaults.MaxResults = 10
	}
	if c.Routing.Strategy == "" {
		c.Routing.Strategy = "round-robin"
	}
	if c.CircuitBreaker.Threshold == 0 {
		c.CircuitBreaker.Threshold = 5
	}
	if c.CircuitBreaker.Timeout == 0 {
		c.CircuitBreaker.Timeout = 30 * time.Second
	}
	if c.CircuitBreaker.Window == 0 {
		c.CircuitBreaker.Window = time.Minute
	}
	if c.CircuitBreaker.Cooldown == 0 {
		c.CircuitBreaker.Cooldown = c.CircuitBreaker.Timeout
	}
	if c.Metrics.Path == "" {
		c.Metrics.Path = "/metrics"
	}
}

// Load reads a YAML config file, performs environment variable substitution,
// applies defaults, and returns the parsed Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	data = substituteEnv(data)

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: invalid configuration")
	}
	// Reject legacy secrets without retaining their values in the configuration.
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parsing config YAML: invalid configuration")
	}
	if containsAPIKey(&document) {
		return nil, fmt.Errorf("config: upstream api_key settings are forbidden; use POST credentials")
	}

	cfg.applyDefaults()

	return &cfg, nil
}

// Validate checks that the config has all required fields set correctly.
func (c *Config) Validate() error {
	if len(c.Auth.TokenHashes) == 0 {
		return fmt.Errorf("config: auth.token_hashes requires at least one gateway token hash")
	}
	for _, hash := range c.Auth.TokenHashes {
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("config: gateway token hashes must be SHA-256 hex strings")
		}
	}
	if c.CircuitBreaker.Threshold < 0 || c.CircuitBreaker.Window < 0 || c.CircuitBreaker.Cooldown < 0 || c.CircuitBreaker.Timeout < 0 {
		return fmt.Errorf("config: circuit breaker settings must be positive")
	}
	if len(c.Providers) == 0 {
		return fmt.Errorf("config: at least one provider must be configured")
	}

	for i, p := range c.Providers {
		if p.Name == "" {
			return fmt.Errorf("config: provider[%d]: name is required", i)
		}
		if p.Tier == 0 {
			return fmt.Errorf("config: provider[%d] (%s): tier is required", i, p.Name)
		}
	}

	return nil
}

func containsAPIKey(node *yaml.Node) bool {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == "api_key" {
				return true
			}
		}
	}
	for _, child := range node.Content {
		if containsAPIKey(child) {
			return true
		}
	}
	return false
}
