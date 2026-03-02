package formatter

import (
	"encoding/json"

	"github.com/johnuopini/searchmux/internal/provider"
)

type searxngResponse struct {
	Query           string            `json:"query"`
	NumberOfResults int               `json:"number_of_results"`
	Results         []provider.Result `json:"results"`
	Infoboxes       []interface{}     `json:"infoboxes"`
	XProvider       string            `json:"x-provider"`
	XTier           int               `json:"x-tier"`
	XFallback       bool              `json:"x-fallback"`
	XLatencyMs      int64             `json:"x-latency-ms"`
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
	if results == nil {
		results = []provider.Result{}
	}
	return json.Marshal(searxngResponse{
		Query:           query,
		NumberOfResults: len(results),
		Results:         results,
		Infoboxes:       []interface{}{},
		XProvider:       meta.ProviderUsed,
		XTier:           meta.TierUsed,
		XFallback:       meta.Fallback,
		XLatencyMs:      meta.Latency.Milliseconds(),
	})
}
