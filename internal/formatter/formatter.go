package formatter

import (
	"fmt"
	"time"

	"github.com/frenzy/searchmux/internal/provider"
)

// Metadata holds information about the search routing decision.
type Metadata struct {
	ProviderUsed string
	TierUsed     int
	Fallback     bool
	Latency      time.Duration
}

// Formatter defines the interface for formatting search results.
type Formatter interface {
	Name() string
	Format(query string, results []provider.Result, meta Metadata) ([]byte, error)
	ContentType() string
}

// Registry stores named formatters and retrieves them by name.
type Registry struct {
	formatters map[string]Formatter
}

// NewRegistry creates a new formatter Registry.
func NewRegistry() *Registry {
	return &Registry{formatters: make(map[string]Formatter)}
}

// Register adds a formatter to the registry.
func (r *Registry) Register(f Formatter) {
	r.formatters[f.Name()] = f
}

// Get retrieves a formatter by name.
func (r *Registry) Get(name string) (Formatter, error) {
	f, ok := r.formatters[name]
	if !ok {
		return nil, fmt.Errorf("unknown formatter: %q", name)
	}
	return f, nil
}
