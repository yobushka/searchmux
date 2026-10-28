package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnuopini/searchmux/internal/formatter"
	"github.com/johnuopini/searchmux/internal/provider"
	"github.com/johnuopini/searchmux/internal/router"
)

func gatewayHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TestGatewayAuth(t *testing.T) {
	for _, header := range []string{"", "Basic tenant-a", "Bearer wrong", "Bearer", "Bearer tenant-a extra", "Bearer tenant-a", "bearer tenant-b"} {
		t.Run(header, func(t *testing.T) {
			called := false
			h := AuthMiddleware([]string{gatewayHash("tenant-a"), gatewayHash("tenant-b")}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) }))
			req := httptest.NewRequest(http.MethodGet, "/search?q=test", nil)
			req.Header.Set("Authorization", header)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			want := header == "Bearer tenant-a" || header == "bearer tenant-b"
			if called != want {
				t.Fatalf("authenticated=%v want=%v", called, want)
			}
			if !want && (rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != "Bearer") {
				t.Fatal("invalid auth response")
			}
		})
	}
}

func TestGatewayAuthEmptyAndDuplicateHeaders(t *testing.T) {
	for _, hashes := range [][]string{nil, {gatewayHash("tenant-a")}} {
		h := AuthMiddleware(hashes, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unauthorized handler invoked") }))
		req := httptest.NewRequest("GET", "/stats", nil)
		req.Header.Add("Authorization", "Bearer tenant-a")
		if hashes != nil {
			req.Header.Add("Authorization", "Bearer tenant-a")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Fatalf("status=%d", rec.Code)
		}
	}
}

// Exercise a real gateway HTTP server, router, and Brave adapter together.
func TestBYOKHTTPIsolationAndStats(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.Header.Get("X-Subscription-Token"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"web":{"results":[{"title":"result","url":"https://example.com","description":"snippet"}]}}`)
	}))
	defer upstream.Close()
	p := provider.NewBrave(time.Second)
	p.BaseURL = upstream.URL
	r := router.New(router.Config{CBThreshold: 1})
	r.AddProvider(p, 1)
	fmts := formatter.NewRegistry()
	fmts.Register(formatter.NewSearXNG())
	h := NewHandler(r, fmts, "searxng")
	gateway := httptest.NewServer(AuthMiddleware([]string{gatewayHash("tenant-a"), gatewayHash("tenant-b")}, h.Mux()))
	defer gateway.Close()
	for _, tenant := range []string{"a", "b"} {
		body := `{"version":1,"q":"test","credentials":{"brave":{"api_key":"key-` + tenant + `"},"exa":{"api_key":"not-for-brave"}},"options":{"max_results":5},"format":"searxng"}`
		req, err := http.NewRequest("POST", gateway.URL+"/search", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer tenant-"+tenant)
		resp, err := gateway.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Results []provider.Result `json:"results"`
		}
		err = json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 || len(data.Results) != 1 {
			t.Fatalf("search status=%d decode=%v", resp.StatusCode, err)
		}
	}
	mu.Lock()
	if len(keys) != 2 || keys[0] != "key-a" || keys[1] != "key-b" {
		t.Errorf("request keys not isolated")
	}
	mu.Unlock()
	// A subsequent GET has no credentials and must not reuse either key.
	for _, path := range []string{"/search?q=test", "/stats", "/metrics"} {
		req, err := http.NewRequest("GET", gateway.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer tenant-a")
		resp, err := gateway.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "key-a") || strings.Contains(string(data), "key-b") {
			t.Fatal("credentials exposed")
		}
		switch path {
		case "/search?q=test":
			if resp.StatusCode != 503 {
				t.Fatal("GET reused a credential")
			}
		case "/stats":
			var snapshot struct {
				Providers map[string]router.ProviderStats `json:"providers"`
			}
			if err := json.Unmarshal(data, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.Providers["brave"].Requests != 2 || snapshot.Providers["brave"].SuccessRate != 1 {
				t.Fatal("wrong stats")
			}
		case "/metrics":
			for _, metric := range []string{`searchmux_provider_up{provider="brave"} 1`, `searchmux_circuit_breaker_state{provider="brave"} 0`, `searchmux_provider_requests_total{provider="brave"} 2`} {
				if !strings.Contains(string(data), metric) {
					t.Errorf("missing metric %s", metric)
				}
			}
		}
	}
}

func TestPOSTSearchValidation(t *testing.T) {
	for _, body := range []string{`{`, `{"version":2,"q":"test"}`, `{"version":1}`, `{"version":1,"q":"test"} {}`, `{"version":1,"q":"test","options":{"max_results":-1}}`, `{"version":1,"q":"test","credentials":{"brave":{"api_key":"\r\nsecret"}}}`, `{"version":1,"q":"test","secret-canary":"value"}`, `{"version":1,"q":"test","format":"secret-canary"}`} {
		h := newTestHandler(&testProvider{name: "mock", available: true})
		req := httptest.NewRequest("POST", "/search", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Mux().ServeHTTP(rec, req)
		if rec.Code != 400 || strings.Contains(rec.Body.String(), "secret-canary") {
			t.Fatalf("unsafe validation status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestLoggingDoesNotRetainCredentials(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	h := LoggingMiddleware(AuthMiddleware([]string{gatewayHash("gateway-canary")}, newTestHandler().Mux()))
	req := httptest.NewRequest("POST", "/search?q=query-canary", strings.NewReader(`{"version":1,"q":"body-canary","credentials":{"brave":{"api_key":"key-canary"}}}`))
	req.Header.Set("Authorization", "Bearer gateway-canary")
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)
	for _, canary := range []string{"gateway-canary", "query-canary", "body-canary", "key-canary"} {
		if strings.Contains(logs.String(), canary) {
			t.Fatalf("logs contain %s", canary)
		}
	}
}
