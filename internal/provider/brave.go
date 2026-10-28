package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Brave implements Provider for the Brave Search API.
type Brave struct {
	client  *http.Client
	BaseURL string
}

// NewBrave creates a new Brave provider.
func NewBrave(timeout time.Duration) *Brave {
	return &Brave{
		client:  newHTTPClient(timeout),
		BaseURL: "https://api.search.brave.com",
	}
}

func (b *Brave) Name() string    { return "brave" }
func (b *Brave) Available() bool { return b.BaseURL != "" }

// braveResponse is the JSON response from the Brave Search API.
type braveResponse struct {
	Web *braveWebResults `json:"web"`
}

type braveWebResults struct {
	Results []braveResult `json:"results"`
}

type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// safesearchMap maps numeric safesearch levels to Brave API values.
var safesearchMap = map[int]string{
	0: "off",
	1: "moderate",
	2: "strict",
}

func (b *Brave) RequiresAPIKey() bool { return true }

func (b *Brave) Search(ctx context.Context, query string, opts SearchOpts) (*Response, error) {
	if opts.APIKey == "" {
		return nil, ErrMissingAPIKey
	}
	url := b.BaseURL + "/res/v1/web/search"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("brave: create request")
	}

	req.Header.Set("X-Subscription-Token", opts.APIKey)

	q := req.URL.Query()
	q.Set("q", query)
	if opts.MaxResults > 0 {
		q.Set("count", strconv.Itoa(opts.MaxResults))
	}
	if ss, ok := safesearchMap[opts.SafeSearch]; ok {
		q.Set("safesearch", ss)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave: do request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}

	var braveResp braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, fmt.Errorf("brave: decode response")
	}

	if braveResp.Web == nil {
		return &Response{Results: []Result{}}, nil
	}

	results := make([]Result, len(braveResp.Web.Results))
	for i, r := range braveResp.Web.Results {
		results[i] = Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
			Source:  "brave",
		}
	}

	return &Response{Results: results}, nil
}
