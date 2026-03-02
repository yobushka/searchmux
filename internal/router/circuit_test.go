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
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	if cb.State() != StateHalfOpen {
		t.Errorf("State = %v", cb.State())
	}
	if !cb.Allow() {
		t.Error("half-open should allow probe")
	}
}

func TestCircuitBreakerClosesOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	cb.Allow()
	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Errorf("State = %v", cb.State())
	}
}

func TestCircuitBreakerReopensOnHalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	cb.Allow()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("State = %v", cb.State())
	}
}

func TestCircuitBreakerResetOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()
	cb.RecordFailure()
	if !cb.Allow() {
		t.Error("should allow -- success reset count")
	}
}
