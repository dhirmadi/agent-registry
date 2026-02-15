package gateway

import (
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedAllowsRequests(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 3, OpenDuration: 1 * time.Second}

	if !cb.Allow("test-server", cfg) {
		t.Error("closed circuit should allow requests")
	}
}

func TestCircuitBreaker_OpenRejectsRequests(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 2, OpenDuration: 1 * time.Second}

	// Record failures to open circuit
	cb.RecordFailure("test-server", cfg)
	cb.RecordFailure("test-server", cfg)

	if cb.State("test-server") != CircuitOpen {
		t.Error("circuit should be open after reaching threshold")
	}

	if cb.Allow("test-server", cfg) {
		t.Error("open circuit should reject requests")
	}
}

func TestCircuitBreaker_FailuresBelowThreshold(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 3, OpenDuration: 1 * time.Second}

	cb.RecordFailure("test-server", cfg)
	cb.RecordFailure("test-server", cfg)

	if cb.State("test-server") != CircuitClosed {
		t.Error("circuit should remain closed below threshold")
	}

	if !cb.Allow("test-server", cfg) {
		t.Error("circuit below threshold should allow requests")
	}
}

func TestCircuitBreaker_TransitionToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 1, OpenDuration: 100 * time.Millisecond}

	// Open circuit
	cb.RecordFailure("test-server", cfg)

	// Wait for open duration
	time.Sleep(150 * time.Millisecond)

	// Should transition to half-open and allow probe
	if !cb.Allow("test-server", cfg) {
		t.Error("circuit should allow probe after open duration")
	}

	if cb.State("test-server") != CircuitHalfOpen {
		t.Error("circuit should be half-open after first probe")
	}
}

func TestCircuitBreaker_HalfOpenRejectsSecondRequest(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 1, OpenDuration: 50 * time.Millisecond}

	// Open -> HalfOpen
	cb.RecordFailure("test-server", cfg)
	time.Sleep(60 * time.Millisecond)
	cb.Allow("test-server", cfg) // Transition to half-open

	// Second request in half-open should be rejected
	if cb.Allow("test-server", cfg) {
		t.Error("half-open circuit should reject additional requests while probe is pending")
	}
}

func TestCircuitBreaker_HalfOpenProbeSuccess(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 1, OpenDuration: 50 * time.Millisecond}

	// Open -> HalfOpen
	cb.RecordFailure("test-server", cfg)
	time.Sleep(60 * time.Millisecond)
	cb.Allow("test-server", cfg) // Transition to half-open

	// Probe succeeds
	cb.RecordSuccess("test-server")

	if cb.State("test-server") != CircuitClosed {
		t.Error("successful probe should close circuit")
	}
}

func TestCircuitBreaker_HalfOpenProbeFailure(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 1, OpenDuration: 50 * time.Millisecond}

	// Open -> HalfOpen
	cb.RecordFailure("test-server", cfg)
	time.Sleep(60 * time.Millisecond)
	cb.Allow("test-server", cfg)

	// Probe fails
	cb.RecordFailure("test-server", cfg)

	if cb.State("test-server") != CircuitOpen {
		t.Error("failed probe should reopen circuit")
	}
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 3, OpenDuration: 1 * time.Second}

	// Accumulate some failures
	cb.RecordFailure("test-server", cfg)
	cb.RecordFailure("test-server", cfg)

	// Success resets
	cb.RecordSuccess("test-server")

	// One more failure should not open since counter was reset
	cb.RecordFailure("test-server", cfg)

	if cb.State("test-server") != CircuitClosed {
		t.Error("circuit should remain closed after success reset + one failure")
	}
}

func TestCircuitBreaker_IndependentLabels(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 1, OpenDuration: 1 * time.Second}

	// Open server1
	cb.RecordFailure("server1", cfg)

	if cb.State("server1") != CircuitOpen {
		t.Error("server1 should be open")
	}

	// server2 should still be closed
	if !cb.Allow("server2", cfg) {
		t.Error("independent labels should have independent state")
	}

	if cb.State("server2") != CircuitClosed {
		t.Error("server2 should remain closed")
	}
}

func TestCircuitBreaker_UnknownLabelIsClosed(t *testing.T) {
	cb := NewCircuitBreaker()

	if cb.State("never-seen") != CircuitClosed {
		t.Error("unknown label should report CircuitClosed")
	}
}

func TestCircuitBreaker_RecordSuccessOnUnknownLabel(t *testing.T) {
	cb := NewCircuitBreaker()

	// Should not panic
	cb.RecordSuccess("unknown")

	if cb.State("unknown") != CircuitClosed {
		t.Error("recording success on unknown label should be safe")
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	// Use -race flag to detect data races
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 5, OpenDuration: 100 * time.Millisecond}

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				cb.Allow("concurrent-test", cfg)
				if j%10 == 0 {
					cb.RecordFailure("concurrent-test", cfg)
				} else {
					cb.RecordSuccess("concurrent-test")
				}
				cb.State("concurrent-test")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker()
	cfg := CircuitBreakerConfig{FailThreshold: 1, OpenDuration: 1 * time.Second}

	cb.RecordFailure("test-server", cfg)

	if cb.State("test-server") != CircuitOpen {
		t.Error("circuit should be open before reset")
	}

	cb.Reset()

	if cb.State("test-server") != CircuitClosed {
		t.Error("reset should clear all state")
	}
}
