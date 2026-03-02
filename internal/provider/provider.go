package provider

import "context"

// Result represents a single search result.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"content"` // "content" to match SearXNG format
	Source  string `json:"engine"`
}

// Response holds the aggregated search results from a provider.
type Response struct {
	Results []Result `json:"results"`
}

// SearchOpts contains optional parameters for a search request.
type SearchOpts struct {
	MaxResults int
	Language   string
	SafeSearch int // 0=off, 1=moderate, 2=strict
}

// Provider defines the interface that all search providers must implement.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string, opts SearchOpts) (*Response, error)
	Available() bool
}
