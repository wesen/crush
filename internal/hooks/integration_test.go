package hooks

import (
	"context"
	"errors"
	"testing"
	"time"
	
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
)

// TestHook is a simple test hook for integration testing
type TestHook struct {
	name         string
	shouldFail   bool
	shouldTimeout bool
}

func (h *TestHook) Info() HookInfo {
	return HookInfo{
		Name:        h.name,
		Version:     "1.0.0",
		Description: "Test hook for circuit breaker integration",
	}
}

func (h *TestHook) BeforeLLMCall(ctx context.Context, c *LLMCallCtx) {
	if h.shouldTimeout {
		time.Sleep(200 * time.Millisecond)
		return
	}
	if h.shouldFail {
		panic("test hook failure")
	}
}

func TestIntegration_CircuitBreakerWithManager(t *testing.T) {
	// Create circuit breaker config for fast testing
	defaultConfig := CircuitBreakerConfig{
		FailureThreshold:     2,     // Open after 2 failures
		Timeout:              50,    // 50ms timeout
		RecoveryTimeout:      100,   // 100ms recovery
		HalfOpenMaxCalls:     2,
		SuccessThreshold:     1,
	}
	
	perHookConfig := map[string]CircuitBreakerConfig{
		"sensitive-hook": {
			FailureThreshold:     1,     // More sensitive
			Timeout:              30,    // Shorter timeout
			RecoveryTimeout:      50,
			HalfOpenMaxCalls:     1,
			SuccessThreshold:     1,
		},
	}
	
	// Create manager with circuit breaker config
	manager := NewWithCircuitBreakerConfig(defaultConfig, perHookConfig)
	
	// Add normal hook
	normalHook := &TestHook{name: "normal-hook", shouldFail: false}
	manager.Add(normalHook)
	
	// Add failing hook
	failingHook := &TestHook{name: "failing-hook", shouldFail: true}
	manager.Add(failingHook)
	
	// Add timeout hook
	timeoutHook := &TestHook{name: "timeout-hook", shouldTimeout: true}
	manager.Add(timeoutHook)
	
	// Add sensitive hook
	sensitiveHook := &TestHook{name: "sensitive-hook", shouldFail: true}
	manager.Add(sensitiveHook)
	
	ctx := context.Background()
	llmCtx := &LLMCallCtx{}
	
	// Test normal operation
	manager.EmitBeforeLLM(ctx, llmCtx)
	
	// Check initial health - all should be healthy
	health := manager.GetHookHealthStatus()
	if !health["normal-hook"] {
		t.Error("Normal hook should be healthy")
	}
	if !health["failing-hook"] {
		t.Error("Failing hook should initially be healthy")
	}
	
	// Test with failures - this should open circuits
	manager.EmitBeforeLLM(ctx, llmCtx) // First failure
	manager.EmitBeforeLLM(ctx, llmCtx) // Second failure - should open failing-hook
	manager.EmitBeforeLLM(ctx, llmCtx) // Third call - sensitive-hook should open after 1 failure
	
	// Check health after failures
	health = manager.GetHookHealthStatus()
	if !health["normal-hook"] {
		t.Error("Normal hook should still be healthy")
	}
	if health["failing-hook"] {
		t.Error("Failing hook should be unhealthy (circuit open)")
	}
	if health["sensitive-hook"] {
		t.Error("Sensitive hook should be unhealthy (circuit open)")
	}
	if health["timeout-hook"] {
		t.Error("Timeout hook should be unhealthy (circuit open)")
	}
	
	// Check metrics
	metrics := manager.GetCircuitBreakerMetrics()
	
	normalMetrics := metrics["normal-hook"]
	if normalMetrics.TotalCalls < 3 {
		t.Errorf("Expected at least 3 calls for normal hook, got %d", normalMetrics.TotalCalls)
	}
	if normalMetrics.FailedCalls > 0 {
		t.Errorf("Expected 0 failures for normal hook, got %d", normalMetrics.FailedCalls)
	}
	
	failingMetrics := metrics["failing-hook"]
	if failingMetrics.FailedCalls < 2 {
		t.Errorf("Expected at least 2 failures for failing hook, got %d", failingMetrics.FailedCalls)
	}
	
	// Reset a specific hook
	success := manager.ResetHookCircuitBreaker("failing-hook")
	if !success {
		t.Error("Failed to reset failing hook circuit breaker")
	}
	
	// Check that reset hook is healthy again
	health = manager.GetHookHealthStatus()
	if !health["failing-hook"] {
		t.Error("Failing hook should be healthy after reset")
	}
	
	// Test recovery for timeout hook
	timeoutHook.shouldTimeout = false // Fix the timeout issue
	
	// Wait for recovery timeout
	time.Sleep(120 * time.Millisecond)
	
	// This should trigger half-open state and potentially close the circuit
	manager.EmitBeforeLLM(ctx, llmCtx)
	
	// Check if timeout hook recovered
	health = manager.GetHookHealthStatus()
	if !health["timeout-hook"] {
		t.Error("Timeout hook should have recovered")
	}
}

func TestIntegration_TransformManagerWithCircuitBreaker(t *testing.T) {
	// Create transform manager with circuit breaker
	defaultConfig := CircuitBreakerConfig{
		FailureThreshold:     1,
		Timeout:              50,
		RecoveryTimeout:      100,
		HalfOpenMaxCalls:     1,
		SuccessThreshold:     1,
	}
	
	serviceReg := &mockServiceRegistry{}
	transformMgr := NewTransformManagerWithCircuitBreakerConfig(serviceReg, defaultConfig, nil)
	
	// Add transform hook that will fail
	failingTransformHook := &TestTransformHook{
		name:       "failing-transform",
		shouldFail: true,
	}
	
	err := transformMgr.Add(failingTransformHook)
	if err != nil {
		t.Fatalf("Failed to add transform hook: %v", err)
	}
	
	// Test transform execution with failure
	ctx := context.Background()
	transformCtx := &ToolTransformContext{
		SessionHookContext: &SessionHookContext{
			SessionID: "test-session",
			AgentID:   "test-agent",
		},
		ToolName: "test-tool",
	}
	
	// This should fail and open the circuit
	err = transformMgr.ExecuteTransformAfterTool(ctx, transformCtx)
	if err != nil {
		t.Errorf("Transform execution should not return error (circuit breaker handles it): %v", err)
	}
	
	// Check that circuit is open
	cb := transformMgr.circuitBreakerManager.GetCircuitBreaker("failing-transform")
	if cb.State() != Open {
		t.Errorf("Expected circuit to be open, got %v", cb.State())
	}
	
	// Reset and test recovery
	cb.Reset()
	failingTransformHook.shouldFail = false
	
	err = transformMgr.ExecuteTransformAfterTool(ctx, transformCtx)
	if err != nil {
		t.Errorf("Transform execution should succeed after reset and fix: %v", err)
	}
	
	// Circuit should be closed again
	if cb.State() != Closed {
		t.Errorf("Expected circuit to be closed after successful execution, got %v", cb.State())
	}
}

// TestTransformHook for transform manager testing
type TestTransformHook struct {
	name       string
	shouldFail bool
}

func (h *TestTransformHook) Info() HookInfo {
	return HookInfo{
		Name:        h.name,
		Version:     "1.0.0",
		Description: "Test transform hook",
	}
}

func (h *TestTransformHook) TransformAfterTool(ctx context.Context, result *ToolTransformContext) (TransformResult[ToolTransformContext], error) {
	if h.shouldFail {
		return TransformResult[ToolTransformContext]{}, errors.New("transform hook failure")
	}
	
	// Return a successful transform
	modifiedCtx := *result
	modifiedCtx.ToolName = result.ToolName + " [transformed]"
	
	return TransformResult[ToolTransformContext]{
		Modified: &modifiedCtx,
		Metadata: map[string]any{
			"transformed_by": h.name,
		},
	}, nil
}

// Mock service registry for testing
type mockServiceRegistry struct{}

func (r *mockServiceRegistry) MessageService() message.Service { return nil }
func (r *mockServiceRegistry) SessionService() session.Service { return nil }
