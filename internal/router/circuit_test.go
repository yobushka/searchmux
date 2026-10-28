package router

import (
	"testing"
	"time"
)

func TestCircuitBreakerStartsClosed(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)
	if !cb.Allow() {
		t.Error("should allow")
	}
	if cb.State() != StateClosed {
		t.Errorf("State = %v", cb.State())
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.Allow() {
		t.Error("should allow before threshold")
	}
	cb.RecordFailure()
	if cb.Allow() {
		t.Error("should deny after threshold")
	}
	if cb.State() != StateOpen {
		t.Errorf("State = %v", cb.State())
	}
}

func TestCircuitBreakerHalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)
	now := time.Unix(100, 0)
	cb.now = func() time.Time { return now }
	cb.RecordFailure()
	cb.RecordFailure()
	now = now.Add(60 * time.Millisecond)
	if cb.State() != StateHalfOpen {
		t.Errorf("State = %v", cb.State())
	}
	if !cb.Allow() {
		t.Error("half-open should allow probe")
	}
}

func TestCircuitBreakerClosesOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)
	now := time.Unix(100, 0)
	cb.now = func() time.Time { return now }
	cb.RecordFailure()
	cb.RecordFailure()
	now = now.Add(60 * time.Millisecond)
	cb.Allow()
	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Errorf("State = %v", cb.State())
	}
}

func TestCircuitBreakerReopensOnHalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)
	now := time.Unix(100, 0)
	cb.now = func() time.Time { return now }
	cb.RecordFailure()
	cb.RecordFailure()
	now = now.Add(60 * time.Millisecond)
	cb.Allow()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("State = %v", cb.State())
	}
}

func TestCircuitBreakerCountsFailuresAcrossSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()
	cb.RecordFailure()
	if cb.Allow() {
		t.Error("success must not erase failures inside the window")
	}
}

func TestCircuitBreakerExpiresFailuresAtWindowBoundary(t *testing.T) {
	now := time.Unix(100, 0)
	cb := NewCircuitBreaker(2, time.Second, time.Minute)
	cb.now = func() time.Time { return now }
	cb.RecordFailure()
	now = now.Add(time.Minute)
	cb.RecordFailure()
	if !cb.Allow() {
		t.Fatal("expired failure counted")
	}
}

func TestCircuitBreakerSingleProbeAndStaleCompletion(t *testing.T) {
	now := time.Unix(100, 0)
	cb := NewCircuitBreaker(1, time.Second)
	cb.now = func() time.Time { return now }
	old, _ := cb.acquire()
	cb.RecordFailure()
	now = now.Add(time.Second)
	probe, allowed := cb.acquire()
	if !allowed {
		t.Fatal("probe denied")
	}
	cb.complete(old, false, false)
	if cb.Allow() {
		t.Fatal("second probe allowed")
	}
	if cb.State() != StateHalfOpen {
		t.Fatal("stale success changed state")
	}
	cb.complete(probe, false, false)
	if cb.State() != StateClosed {
		t.Fatal("probe did not recover")
	}
}
