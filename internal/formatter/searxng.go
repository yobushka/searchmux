package formatter

import (
	"encoding/json"

	"github.com/frenzy/searchmux/internal/provider"
)

type searxngResponse struct {
	Query        string            `json:"query"`
	Results      []provider.Result `json:"results"`
	ProviderUsed string            `json:"provider_used"`
	TierUsed     int               `json:"tier_used"`
	Fallback     bool              `json:"fallback"`
}

// SearXNGFormatter formats search results as SearXNG-compatible JSON.
type SearXNGFormatter struct{}

// NewSearXNG creates a new SearXNG formatter.
func NewSearXNG() *SearXNGFormatter { return &SearXNGFormatter{} }

// Name returns the formatter name.
func (f *SearXNGFormatter) Name() string { return "searxng" }

// ContentType returns the MIME content type for the output.
func (f *SearXNGFormatter) ContentType() string { return "application/json" }

// Format serializes the search results into SearXNG JSON format.
func (f *SearXNGFormatter) Format(query string, results []provider.Result, meta Metadata) ([]byte, error) {
	return json.Marshal(searxngResponse{
		Query:        query,
		Results:      results,
		ProviderUsed: meta.ProviderUsed,
		TierUsed:     meta.TierUsed,
		Fallback:     meta.Fallback,
	})
}
