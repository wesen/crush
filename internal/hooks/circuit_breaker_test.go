package hooks

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_BasicOperation(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:     2,
		Timeout:              100,  // 100ms
		RecoveryTimeout:      200,  // 200ms
		HalfOpenMaxCalls:     2,
		SuccessThreshold:     1,
	}
	
	cb := NewCircuitBreaker("test-hook", config)
	
	// Initially circuit should be closed
	if cb.State() != Closed {
		t.Fatalf("Expected circuit to be Closed, got %v", cb.State())
	}
	
	// Successful calls should keep circuit closed
	err := cb.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Fatalf("Expected successful call, got error: %v", err)
	}
	
	if cb.State() != Closed {
		t.Fatalf("Expected circuit to remain Closed after success, got %v", cb.State())
	}
}

func TestCircuitBreaker_FailureThreshold(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:     2,
		Timeout:              100,
		RecoveryTimeout:      200,
		HalfOpenMaxCalls:     2,
		SuccessThreshold:     1,
	}
	
	cb := NewCircuitBreaker("test-hook", config)
	testErr := errors.New("test error")
	
	// First failure - should stay closed
	err := cb.Execute(context.Background(), func() error { return testErr })
	if err == nil || err.Error() != testErr.Error() {
		t.Fatalf("Expected test error, got: %v", err)
	}
	
	if cb.State() != Closed {
		t.Fatalf("Expected circuit to remain Closed after first failure, got %v", cb.State())
	}
	
	// Second failure - should open circuit
	err = cb.Execute(context.Background(), func() error { return testErr })
	if err == nil || err.Error() != testErr.Error() {
		t.Fatalf("Expected test error, got: %v", err)
	}
	
	if cb.State() != Open {
		t.Fatalf("Expected circuit to be Open after hitting failure threshold, got %v", cb.State())
	}
	
	// Next call should be rejected immediately
	err = cb.Execute(context.Background(), func() error { return nil })
	if err == nil || err.Error() != "circuit breaker open for hook test-hook" {
		t.Fatalf("Expected circuit open error, got: %v", err)
	}
}

func TestCircuitBreaker_Timeout(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:     2,
		Timeout:              50,   // 50ms timeout
		RecoveryTimeout:      200,
		HalfOpenMaxCalls:     2,
		SuccessThreshold:     1,
	}
	
	cb := NewCircuitBreaker("test-hook", config)
	
	// Function that takes longer than timeout
	err := cb.Execute(context.Background(), func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	
	if err == nil || err.Error() != "hook test-hook timed out after 50ms" {
		t.Fatalf("Expected timeout error, got: %v", err)
	}
	
	// Timeout should count as failure
	metrics := cb.Metrics()
	if metrics.TimeoutCalls != 1 {
		t.Fatalf("Expected 1 timeout call, got %d", metrics.TimeoutCalls)
	}
}

func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:     1,     // Open after 1 failure
		Timeout:              100,
		RecoveryTimeout:      50,    // Short recovery time for testing
		HalfOpenMaxCalls:     2,
		SuccessThreshold:     1,     // Need 1 success to close
	}
	
	cb := NewCircuitBreaker("test-hook", config)
	testErr := errors.New("test error")
	
	// Cause circuit to open
	err := cb.Execute(context.Background(), func() error { return testErr })
	if err == nil {
		t.Fatal("Expected failure")
	}
	
	if cb.State() != Open {
		t.Fatalf("Expected circuit to be Open, got %v", cb.State())
	}
	
	// Wait for recovery timeout
	time.Sleep(60 * time.Millisecond)
	
	// Next call should transition to half-open
	err = cb.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Fatalf("Expected successful call in half-open, got: %v", err)
	}
	
	if cb.State() != Closed {
		t.Fatalf("Expected circuit to be Closed after successful half-open call, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:     1,
		Timeout:              100,
		RecoveryTimeout:      50,
		HalfOpenMaxCalls:     2,
		SuccessThreshold:     1,
	}
	
	cb := NewCircuitBreaker("test-hook", config)
	testErr := errors.New("test error")
	
	// Cause circuit to open
	cb.Execute(context.Background(), func() error { return testErr })
	
	// Wait for recovery timeout
	time.Sleep(60 * time.Millisecond)
	
	// Fail in half-open state should reopen circuit
	err := cb.Execute(context.Background(), func() error { return testErr })
	if err == nil {
		t.Fatal("Expected failure")
	}
	
	if cb.State() != Open {
		t.Fatalf("Expected circuit to be Open after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreaker_Metrics(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.Timeout = 50 // Short timeout for testing
	
	cb := NewCircuitBreaker("test-hook", config)
	testErr := errors.New("test error")
	
	// Successful call
	cb.Execute(context.Background(), func() error { return nil })
	
	// Failed call
	cb.Execute(context.Background(), func() error { return testErr })
	
	// Timeout call
	cb.Execute(context.Background(), func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	
	metrics := cb.Metrics()
	
	if metrics.TotalCalls != 3 {
		t.Fatalf("Expected 3 total calls, got %d", metrics.TotalCalls)
	}
	
	if metrics.SuccessfulCalls != 1 {
		t.Fatalf("Expected 1 successful call, got %d", metrics.SuccessfulCalls)
	}
	
	if metrics.FailedCalls != 1 {
		t.Fatalf("Expected 1 failed call, got %d", metrics.FailedCalls)
	}
	
	if metrics.TimeoutCalls != 1 {
		t.Fatalf("Expected 1 timeout call, got %d", metrics.TimeoutCalls)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:     1,
		Timeout:              100,
		RecoveryTimeout:      1000, // Long recovery time
		HalfOpenMaxCalls:     2,
		SuccessThreshold:     1,
	}
	
	cb := NewCircuitBreaker("test-hook", config)
	testErr := errors.New("test error")
	
	// Cause circuit to open
	cb.Execute(context.Background(), func() error { return testErr })
	
	if cb.State() != Open {
		t.Fatalf("Expected circuit to be Open, got %v", cb.State())
	}
	
	// Reset should close the circuit
	cb.Reset()
	
	if cb.State() != Closed {
		t.Fatalf("Expected circuit to be Closed after reset, got %v", cb.State())
	}
	
	// Should accept calls again
	err := cb.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Fatalf("Expected successful call after reset, got: %v", err)
	}
}

func TestCircuitBreakerManager(t *testing.T) {
	defaultConfig := DefaultCircuitBreakerConfig()
	manager := NewCircuitBreakerManager(defaultConfig)
	
	// Get circuit breaker for hook
	cb1 := manager.GetCircuitBreaker("hook1")
	if cb1 == nil {
		t.Fatal("Expected circuit breaker, got nil")
	}
	
	// Getting same hook should return same instance
	cb2 := manager.GetCircuitBreaker("hook1")
	if cb1 != cb2 {
		t.Fatal("Expected same circuit breaker instance")
	}
	
	// Different hook should get different instance
	cb3 := manager.GetCircuitBreaker("hook2")
	if cb1 == cb3 {
		t.Fatal("Expected different circuit breaker instance")
	}
}

func TestCircuitBreakerManager_PerHookConfig(t *testing.T) {
	defaultConfig := DefaultCircuitBreakerConfig()
	manager := NewCircuitBreakerManager(defaultConfig)
	
	// Set custom config for specific hook
	customConfig := CircuitBreakerConfig{
		FailureThreshold:     1,
		Timeout:              200,
		RecoveryTimeout:      100,
		HalfOpenMaxCalls:     1,
		SuccessThreshold:     1,
	}
	manager.SetHookConfig("special-hook", customConfig)
	
	// Get circuit breaker for configured hook
	cb := manager.GetCircuitBreaker("special-hook")
	
	// Test that it uses custom config (verify by checking behavior)
	testErr := errors.New("test error")
	cb.Execute(context.Background(), func() error { return testErr })
	
	// Should open after 1 failure due to custom config
	if cb.State() != Open {
		t.Fatalf("Expected circuit to be Open with custom config, got %v", cb.State())
	}
}

func TestCircuitBreakerManager_HealthStatus(t *testing.T) {
	manager := NewCircuitBreakerManager(DefaultCircuitBreakerConfig())
	
	// Get some circuit breakers
	cb1 := manager.GetCircuitBreaker("hook1")
	manager.GetCircuitBreaker("hook2") // Ensure hook2 exists
	
	// Open one circuit
	testErr := errors.New("test error")
	for i := 0; i < 3; i++ {
		cb1.Execute(context.Background(), func() error { return testErr })
	}
	
	// Check health status
	status := manager.GetHealthStatus()
	
	if status["hook1"] != false {
		t.Fatal("Expected hook1 to be unhealthy (circuit open)")
	}
	
	if status["hook2"] != true {
		t.Fatal("Expected hook2 to be healthy (circuit closed)")
	}
}

func TestCircuitBreakerManager_Metrics(t *testing.T) {
	manager := NewCircuitBreakerManager(DefaultCircuitBreakerConfig())
	
	// Execute some operations
	cb1 := manager.GetCircuitBreaker("hook1")
	cb1.Execute(context.Background(), func() error { return nil })
	cb1.Execute(context.Background(), func() error { return errors.New("error") })
	
	cb2 := manager.GetCircuitBreaker("hook2")
	cb2.Execute(context.Background(), func() error { return nil })
	
	// Get all metrics
	metrics := manager.GetAllMetrics()
	
	if len(metrics) != 2 {
		t.Fatalf("Expected metrics for 2 hooks, got %d", len(metrics))
	}
	
	if metrics["hook1"].TotalCalls != 2 {
		t.Fatalf("Expected 2 calls for hook1, got %d", metrics["hook1"].TotalCalls)
	}
	
	if metrics["hook2"].TotalCalls != 1 {
		t.Fatalf("Expected 1 call for hook2, got %d", metrics["hook2"].TotalCalls)
	}
}
