package router

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/johnuopini/searchmux/internal/provider"
	"github.com/johnuopini/searchmux/internal/ratelimit"
)

type keyedProvider struct {
	name   string
	status int
	keys   []string
}

func (p *keyedProvider) Name() string         { return p.name }
func (p *keyedProvider) Available() bool      { return true }
func (p *keyedProvider) RequiresAPIKey() bool { return true }
func (p *keyedProvider) Search(_ context.Context, _ string, opts provider.SearchOpts) (*provider.Response, error) {
	p.keys = append(p.keys, opts.APIKey)
	if opts.Credentials != nil {
		return nil, &provider.HTTPError{StatusCode: 500}
	}
	if p.status != 0 {
		return nil, &provider.HTTPError{StatusCode: p.status}
	}
	return &provider.Response{}, nil
}

func TestTenantCredentialFailuresDoNotPoisonProvider(t *testing.T) {
	for _, status := range []int{401, 403, 429} {
		p := &keyedProvider{name: "brave", status: status}
		tracker := ratelimit.New()
		tracker.Configure("brave", 1, time.Hour)
		r := New(Config{CBThreshold: 1, RateLimiter: tracker})
		r.AddProvider(p, 1)
		r.AddProvider(&mockProvider{name: "backup", available: true}, 2)
		result, err := r.Search(context.Background(), "test", provider.SearchOpts{Credentials: map[string]string{"brave": "invalid-tenant-key"}})
		if err != nil || result.ProviderUsed != "backup" {
			t.Fatalf("credential error did not fail over: %v", err)
		}
		if r.Health()["brave"].Circuit != "closed" {
			t.Fatal("tenant error opened provider circuit")
		}
		p.status = 0
		for i := 0; i < 2; i++ {
			result, err = r.Search(context.Background(), "test", provider.SearchOpts{Credentials: map[string]string{"brave": "valid-tenant-key", "exa": "other-provider-key"}})
			if err != nil || result.ProviderUsed != "brave" {
				t.Fatalf("other tenant was blocked: %v", err)
			}
		}
		stats := r.Stats()["brave"]
		if stats.Requests != 3 || stats.Failures != 1 || stats.Successes != 2 || stats.FailureRate != 1.0/3 {
			t.Fatalf("incorrect attempt stats: %+v", stats)
		}
		if p.keys[1] != "valid-tenant-key" {
			t.Fatal("wrong key forwarded")
		}
	}
}

func TestMissingCredentialsSkipWithoutRecordingFailure(t *testing.T) {
	p := &keyedProvider{name: "brave"}
	r := New(Config{CBThreshold: 1})
	r.AddProvider(p, 1)
	_, err := r.Search(context.Background(), "test", provider.SearchOpts{})
	if err == nil || len(p.keys) != 0 || r.Stats()["brave"].Requests != 0 {
		t.Fatal("missing credential triggered an upstream attempt")
	}
}

func TestCircuitBreakerConcurrentProbe(t *testing.T) {
	now := time.Unix(100, 0)
	cb := NewCircuitBreaker(1, time.Second)
	cb.now = func() time.Time { return now }
	cb.RecordFailure()
	now = now.Add(time.Second)
	var wg sync.WaitGroup
	results := make(chan bool, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- cb.Allow() }()
	}
	wg.Wait()
	close(results)
	allowed := 0
	for ok := range results {
		if ok {
			allowed++
		}
	}
	if allowed != 1 {
		t.Fatalf("allowed %d probes", allowed)
	}
}

func TestHealthDoesNotAcquireHalfOpenProbe(t *testing.T) {
	r := New(Config{CBThreshold: 1, CBTimeout: time.Second})
	p := &mockProvider{name: "test", available: true}
	r.AddProvider(p, 1)
	now := time.Unix(100, 0)
	cb := r.entries[0].cb
	cb.now = func() time.Time { return now }
	cb.RecordFailure()
	now = now.Add(time.Second)
	if r.Health()["test"].Circuit != "half-open" || r.Stats()["test"].Circuit != "half-open" {
		t.Fatal("cooldown not observable")
	}
	if _, err := r.Search(context.Background(), "test", provider.SearchOpts{}); err != nil {
		t.Fatal("health consumed probe", err)
	}
	if cb.State() != StateClosed {
		t.Fatal("probe did not close breaker")
	}
}
