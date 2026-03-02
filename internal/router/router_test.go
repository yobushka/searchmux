package router

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/johnuopini/searchmux/internal/provider"
	"github.com/johnuopini/searchmux/internal/ratelimit"
)

// mockProvider is a configurable mock for testing the router.
type mockProvider struct {
	name      string
	available bool
	results   []provider.Result
	err       error
	called    int
}

func (m *mockProvider) Name() string        { return m.name }
func (m *mockProvider) Available() bool      { return m.available }
func (m *mockProvider) Search(ctx context.Context, query string, opts provider.SearchOpts) (*provider.Response, error) {
	m.called++
	if m.err != nil {
		return nil, m.err
	}
	return &provider.Response{Results: m.results}, nil
}

func TestRouterUsesHighestPriorityProvider(t *testing.T) {
	r := New(Config{CBThreshold: 5, CBTimeout: time.Minute})

	tier1 := &mockProvider{
		name: "primary", available: true,
		results: []provider.Result{{Title: "tier1-result"}},
	}
	tier2 := &mockProvider{
		name: "secondary", available: true,
		results: []provider.Result{{Title: "tier2-result"}},
	}

	r.AddProvider(tier1, 1)
	r.AddProvider(tier2, 2)

	res, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.ProviderUsed != "primary" {
		t.Errorf("ProviderUsed = %q, want %q", res.ProviderUsed, "primary")
	}
	if res.TierUsed != 1 {
		t.Errorf("TierUsed = %d, want 1", res.TierUsed)
	}
	if res.Fallback {
		t.Error("Fallback should be false for tier 1")
	}
	if tier1.called != 1 {
		t.Errorf("tier1.called = %d, want 1", tier1.called)
	}
	if tier2.called != 0 {
		t.Errorf("tier2.called = %d, want 0", tier2.called)
	}
}

func TestRouterFallsBackToNextTier(t *testing.T) {
	r := New(Config{CBThreshold: 5, CBTimeout: time.Minute})

	tier1 := &mockProvider{
		name: "primary", available: true,
		err: fmt.Errorf("connection refused"),
	}
	tier2 := &mockProvider{
		name: "secondary", available: true,
		results: []provider.Result{{Title: "fallback-result"}},
	}

	r.AddProvider(tier1, 1)
	r.AddProvider(tier2, 2)

	res, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.ProviderUsed != "secondary" {
		t.Errorf("ProviderUsed = %q, want %q", res.ProviderUsed, "secondary")
	}
	if res.TierUsed != 2 {
		t.Errorf("TierUsed = %d, want 2", res.TierUsed)
	}
	if !res.Fallback {
		t.Error("Fallback should be true when using lower-priority tier")
	}
	if tier1.called != 1 {
		t.Errorf("tier1.called = %d, want 1", tier1.called)
	}
	if tier2.called != 1 {
		t.Errorf("tier2.called = %d, want 1", tier2.called)
	}
}

func TestRouterSkipsUnavailableProviders(t *testing.T) {
	r := New(Config{CBThreshold: 5, CBTimeout: time.Minute})

	unavail := &mockProvider{
		name: "down", available: false,
		results: []provider.Result{{Title: "should-not-see"}},
	}
	avail := &mockProvider{
		name: "up", available: true,
		results: []provider.Result{{Title: "available-result"}},
	}

	r.AddProvider(unavail, 1)
	r.AddProvider(avail, 1)

	res, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.ProviderUsed != "up" {
		t.Errorf("ProviderUsed = %q, want %q", res.ProviderUsed, "up")
	}
	if unavail.called != 0 {
		t.Errorf("unavailable provider was called %d times", unavail.called)
	}
}

func TestRouterAllProvidersExhausted(t *testing.T) {
	r := New(Config{CBThreshold: 5, CBTimeout: time.Minute})

	p1 := &mockProvider{name: "a", available: true, err: fmt.Errorf("fail-a")}
	p2 := &mockProvider{name: "b", available: true, err: fmt.Errorf("fail-b")}

	r.AddProvider(p1, 1)
	r.AddProvider(p2, 2)

	_, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err == nil {
		t.Fatal("Search() should return error when all providers fail")
	}
}

func TestRouterRoundRobinWithinTier(t *testing.T) {
	r := New(Config{CBThreshold: 5, CBTimeout: time.Minute})

	p1 := &mockProvider{
		name: "alpha", available: true,
		results: []provider.Result{{Title: "alpha"}},
	}
	p2 := &mockProvider{
		name: "beta", available: true,
		results: []provider.Result{{Title: "beta"}},
	}

	r.AddProvider(p1, 1)
	r.AddProvider(p2, 1)

	// Make multiple calls and verify alternation.
	providers := make(map[string]int)
	for i := 0; i < 10; i++ {
		res, err := r.Search(context.Background(), "test", provider.SearchOpts{})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		providers[res.ProviderUsed]++
	}

	// Both providers should be used (round-robin distributes evenly).
	if providers["alpha"] == 0 {
		t.Error("alpha was never used")
	}
	if providers["beta"] == 0 {
		t.Error("beta was never used")
	}
	// With round-robin, should be 5/5.
	if providers["alpha"] != 5 || providers["beta"] != 5 {
		t.Errorf("distribution: alpha=%d, beta=%d, want 5/5", providers["alpha"], providers["beta"])
	}
}

func TestRouterWithRateLimiter(t *testing.T) {
	rl := ratelimit.New()
	rl.Configure("limited", 0, time.Minute) // zero limit = immediately limited

	r := New(Config{
		CBThreshold: 5,
		CBTimeout:   time.Minute,
		RateLimiter: rl,
	})

	limited := &mockProvider{
		name: "limited", available: true,
		results: []provider.Result{{Title: "limited-result"}},
	}
	unlimited := &mockProvider{
		name: "unlimited", available: true,
		results: []provider.Result{{Title: "unlimited-result"}},
	}

	r.AddProvider(limited, 1)
	r.AddProvider(unlimited, 1)

	res, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.ProviderUsed != "unlimited" {
		t.Errorf("ProviderUsed = %q, want %q", res.ProviderUsed, "unlimited")
	}
	if limited.called != 0 {
		t.Errorf("rate-limited provider was called %d times", limited.called)
	}
}

func TestRouterCircuitBreakerTrips(t *testing.T) {
	r := New(Config{CBThreshold: 2, CBTimeout: time.Minute})

	failing := &mockProvider{
		name: "fragile", available: true,
		err: fmt.Errorf("always fails"),
	}
	backup := &mockProvider{
		name: "backup", available: true,
		results: []provider.Result{{Title: "backup-result"}},
	}

	r.AddProvider(failing, 1)
	r.AddProvider(backup, 2)

	// First 2 calls should hit fragile (which fails) then fall back to backup.
	for i := 0; i < 2; i++ {
		res, err := r.Search(context.Background(), "test", provider.SearchOpts{})
		if err != nil {
			t.Fatalf("Search() call %d error = %v", i+1, err)
		}
		if res.ProviderUsed != "backup" {
			t.Errorf("call %d: ProviderUsed = %q, want %q", i+1, res.ProviderUsed, "backup")
		}
	}

	// After 2 failures, circuit breaker should be open for "fragile".
	// The third call should skip "fragile" entirely (circuit open) and go to backup.
	failing.called = 0 // reset counter
	res, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err != nil {
		t.Fatalf("Search() after CB trip error = %v", err)
	}
	if res.ProviderUsed != "backup" {
		t.Errorf("ProviderUsed = %q, want %q", res.ProviderUsed, "backup")
	}
	// The failing provider should NOT have been called this time
	// because the circuit breaker is open.
	if failing.called != 0 {
		t.Errorf("fragile.called = %d after CB tripped, want 0", failing.called)
	}
}

func TestRouterProviderHealth(t *testing.T) {
	rl := ratelimit.New()
	rl.Configure("alpha", 100, time.Minute)

	r := New(Config{
		CBThreshold: 3,
		CBTimeout:   time.Minute,
		RateLimiter: rl,
	})

	p1 := &mockProvider{name: "alpha", available: true}
	p2 := &mockProvider{name: "beta", available: false}

	r.AddProvider(p1, 1)
	r.AddProvider(p2, 2)

	health := r.Health()

	if len(health) != 2 {
		t.Fatalf("Health() returned %d entries, want 2", len(health))
	}

	h1, ok := health["alpha"]
	if !ok {
		t.Fatal("Health() missing entry for alpha")
	}
	if !h1.Available {
		t.Error("alpha should be available")
	}
	if h1.Circuit != "closed" {
		t.Errorf("alpha circuit = %q, want %q", h1.Circuit, "closed")
	}
	if h1.RateLimitRemaining != 100 {
		t.Errorf("alpha rate_limit_remaining = %d, want 100", h1.RateLimitRemaining)
	}

	h2, ok := health["beta"]
	if !ok {
		t.Fatal("Health() missing entry for beta")
	}
	if h2.Available {
		t.Error("beta should not be available")
	}
	if h2.Circuit != "closed" {
		t.Errorf("beta circuit = %q, want %q", h2.Circuit, "closed")
	}
	// beta is not configured in rate limiter -> -1
	if h2.RateLimitRemaining != -1 {
		t.Errorf("beta rate_limit_remaining = %d, want -1", h2.RateLimitRemaining)
	}
}

func TestRouterNoProviders(t *testing.T) {
	r := New(Config{CBThreshold: 3, CBTimeout: time.Minute})

	_, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err == nil {
		t.Error("Search() with no providers should return error")
	}
}

func TestRouterSearchResultHasLatency(t *testing.T) {
	r := New(Config{CBThreshold: 5, CBTimeout: time.Minute})

	p := &mockProvider{
		name: "fast", available: true,
		results: []provider.Result{{Title: "result"}},
	}
	r.AddProvider(p, 1)

	res, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Latency <= 0 {
		t.Error("Latency should be positive")
	}
}

func TestRouterRecordsRateLimitOn429(t *testing.T) {
	rl := ratelimit.New()
	rl.Configure("flaky", 10, time.Minute)

	r := New(Config{
		CBThreshold: 10, // high threshold so CB doesn't trip
		CBTimeout:   time.Minute,
		RateLimiter: rl,
	})

	flaky := &mockProvider{
		name: "flaky", available: true,
		err: fmt.Errorf("HTTP 429: rate limit exceeded"),
	}
	backup := &mockProvider{
		name: "backup", available: true,
		results: []provider.Result{{Title: "backup"}},
	}

	r.AddProvider(flaky, 1)
	r.AddProvider(backup, 2)

	_, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	// After a 429 error, the rate limiter should block the provider.
	if rl.Allow("flaky") {
		t.Error("rate limiter should block flaky after 429")
	}
}
