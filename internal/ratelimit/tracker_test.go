package ratelimit

import (
	"testing"
	"time"
)

func TestTrackerAllow(t *testing.T) {
	tr := New()
	tr.Configure("tavily", 3, 1*time.Hour)
	for i := 0; i < 3; i++ {
		if !tr.Allow("tavily") {
			t.Errorf("request %d should be allowed", i+1)
		}
		tr.Record("tavily")
	}
	if tr.Allow("tavily") {
		t.Error("4th request should be denied")
	}
}

func TestTrackerUnconfiguredProvider(t *testing.T) {
	tr := New()
	if !tr.Allow("searxng") {
		t.Error("unconfigured provider should always be allowed")
	}
}

func TestTrackerRecordLimit(t *testing.T) {
	tr := New()
	tr.Configure("brave", 100, 1*time.Hour)
	if !tr.Allow("brave") {
		t.Error("should be allowed initially")
	}
	tr.RecordLimit("brave")
	if tr.Allow("brave") {
		t.Error("should be denied after 429")
	}
}

func TestTrackerWindowReset(t *testing.T) {
	tr := New()
	tr.Configure("tavily", 2, 50*time.Millisecond)
	tr.Record("tavily")
	tr.Record("tavily")
	if tr.Allow("tavily") {
		t.Error("should be denied at limit")
	}
	time.Sleep(60 * time.Millisecond)
	if !tr.Allow("tavily") {
		t.Error("should be allowed after window reset")
	}
}

func TestTrackerRemaining(t *testing.T) {
	tr := New()
	tr.Configure("tavily", 10, 1*time.Hour)
	if r := tr.Remaining("tavily"); r != 10 {
		t.Errorf("Remaining = %d, want 10", r)
	}
	tr.Record("tavily")
	tr.Record("tavily")
	if r := tr.Remaining("tavily"); r != 8 {
		t.Errorf("Remaining = %d, want 8", r)
	}
	if r := tr.Remaining("searxng"); r != -1 {
		t.Errorf("Remaining = %d, want -1 for unconfigured", r)
	}
}
