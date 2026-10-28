package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Exa searches using a key supplied for this request only.
type Exa struct {
	client  *http.Client
	BaseURL string
}

func NewExa(timeout time.Duration) *Exa {
	return &Exa{client: newHTTPClient(timeout), BaseURL: "https://api.exa.ai"}
}
func (e *Exa) Name() string         { return "exa" }
func (e *Exa) Available() bool      { return e.BaseURL != "" }
func (e *Exa) RequiresAPIKey() bool { return true }

func (e *Exa) Search(ctx context.Context, query string, opts SearchOpts) (*Response, error) {
	if opts.APIKey == "" {
		return nil, ErrMissingAPIKey
	}
	payload := struct {
		Query      string `json:"query"`
		NumResults int    `json:"numResults,omitempty"`
		Contents   struct {
			Text bool `json:"text"`
		} `json:"contents"`
	}{Query: query, NumResults: opts.MaxResults}
	payload.Contents.Text = true
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("exa: encode request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("exa: create request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", opts.APIKey)
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exa: request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}
	var data struct {
		Results []struct {
			Title      string   `json:"title"`
			URL        string   `json:"url"`
			Text       string   `json:"text"`
			Highlights []string `json:"highlights"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("exa: invalid response")
	}
	results := make([]Result, 0, len(data.Results))
	for _, item := range data.Results {
		text := item.Text
		if text == "" {
			text = strings.Join(item.Highlights, "\n")
		}
		results = append(results, Result{Title: item.Title, URL: item.URL, Snippet: text, Source: e.Name()})
		if opts.MaxResults > 0 && len(results) == opts.MaxResults {
			break
		}
	}
	return &Response{Results: results}, nil
}
