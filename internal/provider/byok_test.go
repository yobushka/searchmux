package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewAdaptersWireContract(t *testing.T) {
	for _, name := range []string{"exa", "jina"} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("incorrect method or content type")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if name == "exa" {
					if r.URL.Path != "/search" || r.Header.Get("x-api-key") != "request-key" || body["query"] != "test" || body["numResults"] != float64(1) {
						t.Error("incorrect Exa request")
					}
					io.WriteString(w, `{"results":[{"title":"title","url":"https://example.com","text":"snippet"},{"title":"extra"}]}`)
				} else {
					if r.URL.Path != "/" || r.Header.Get("Authorization") != "Bearer request-key" || r.Header.Get("Accept") != "application/json" || body["q"] != "test" {
						t.Error("incorrect Jina request")
					}
					io.WriteString(w, `{"data":[{"title":"title","url":"https://example.com","description":"snippet"},{"title":"extra"}]}`)
				}
			}))
			defer srv.Close()
			var p Provider
			if name == "exa" {
				e := NewExa(time.Second)
				e.BaseURL = srv.URL
				p = e
			} else {
				j := NewJina(time.Second)
				j.BaseURL = srv.URL
				p = j
			}
			resp, err := p.Search(context.Background(), "test", SearchOpts{APIKey: "request-key", MaxResults: 1})
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.Results) != 1 || resp.Results[0] != (Result{Title: "title", URL: "https://example.com", Snippet: "snippet", Source: name}) {
				t.Fatalf("incorrect mapping: %+v", resp)
			}
		})
	}
}

func TestKeyedAdaptersRejectMissingKeysAndSafeErrors(t *testing.T) {
	for _, name := range []string{"brave", "tavily", "serper", "exa", "jina"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(401)
				io.WriteString(w, "secret-canary")
			}))
			defer srv.Close()
			p := keyedTestProvider(name, srv.URL)
			if _, err := p.Search(context.Background(), "test", SearchOpts{}); !errors.Is(err, ErrMissingAPIKey) {
				t.Fatalf("missing key: %v", err)
			}
			if calls != 0 {
				t.Fatal("missing key sent upstream")
			}
			_, err := p.Search(context.Background(), "test", SearchOpts{APIKey: "secret-canary"})
			var status *HTTPError
			if !errors.As(err, &status) || status.StatusCode != 401 || strings.Contains(err.Error(), "secret-canary") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}

func keyedTestProvider(name, url string) Provider {
	switch name {
	case "brave":
		p := NewBrave(time.Second)
		p.BaseURL = url
		return p
	case "tavily":
		p := NewTavily(time.Second)
		p.BaseURL = url
		return p
	case "serper":
		p := NewSerper(time.Second)
		p.BaseURL = url
		return p
	case "exa":
		p := NewExa(time.Second)
		p.BaseURL = url
		return p
	default:
		p := NewJina(time.Second)
		p.BaseURL = url
		return p
	}
}

func TestKeyedAdaptersDoNotFollowRedirects(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("credentials forwarded on redirect") }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	for _, name := range []string{"brave", "tavily", "serper", "exa", "jina"} {
		_, err := keyedTestProvider(name, source.URL).Search(context.Background(), "test", SearchOpts{APIKey: "secret"})
		var status *HTTPError
		if !errors.As(err, &status) || status.StatusCode != 307 {
			t.Fatalf("%s followed redirect: %v", name, err)
		}
	}
}
