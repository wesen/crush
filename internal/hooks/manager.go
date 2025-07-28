package hooks

import (
	"context"
	"log/slog"
	"time"
)

// Manager manages a collection of hooks and emits events to them.
type Manager struct {
	hooks                 []Hook
	circuitBreakerManager *CircuitBreakerManager
	logger                *slog.Logger
}

// New creates a new hook manager.
func New() *Manager {
	logger := slog.Default().With("component", "hook_manager")
	logger.Debug("Creating new hook manager")
	return &Manager{
		hooks:                 make([]Hook, 0),
		circuitBreakerManager: NewCircuitBreakerManager(DefaultCircuitBreakerConfig()),
		logger:                logger,
	}
}

// NewWithCircuitBreakerConfig creates a new hook manager with custom circuit breaker configuration.
func NewWithCircuitBreakerConfig(defaultConfig CircuitBreakerConfig, perHookConfig map[string]CircuitBreakerConfig) *Manager {
	logger := slog.Default().With("component", "hook_manager")
	logger.Debug("Creating new hook manager with custom circuit breaker config",
		"default_config", defaultConfig,
		"per_hook_configs", len(perHookConfig))
	
	cbManager := NewCircuitBreakerManager(defaultConfig)
	for hookName, config := range perHookConfig {
		logger.Debug("Setting per-hook circuit breaker config", "hook", hookName, "config", config)
		cbManager.SetHookConfig(hookName, config)
	}
	return &Manager{
		hooks:                 make([]Hook, 0),
		circuitBreakerManager: cbManager,
		logger:                logger,
	}
}

// Add adds a hook to the manager.
func (m *Manager) Add(h Hook) {
	if m == nil {
		return
	}
	
	info := h.Info()
	m.logger.Debug("Adding hook to manager",
		"hook", info.Name,
		"version", info.Version,
		"description", info.Description,
		"total_hooks", len(m.hooks)+1)
	
	m.hooks = append(m.hooks, h)
	
	m.logger.Info("Hook added to event manager",
		"hook", info.Name,
		"total_hooks", len(m.hooks))
}

// safeCall executes a function with panic recovery and circuit breaker protection.
func (m *Manager) safeCall(hookName string, fn func() error) {
	start := time.Now()
	
	m.logger.Debug("Executing hook", "hook", hookName)
	
	if m.circuitBreakerManager == nil {
		// Fallback to old behavior if no circuit breaker manager
		defer func() {
			duration := time.Since(start)
			if r := recover(); r != nil {
				m.logger.Error("Hook panic recovered", 
					"hook", hookName, 
					"panic", r,
					"duration", duration)
			} else {
				m.logger.Debug("Hook execution completed (no circuit breaker)",
					"hook", hookName,
					"duration", duration)
			}
		}()
		if err := fn(); err != nil {
			m.logger.Warn("Hook execution failed", 
				"hook", hookName, 
				"error", err,
				"duration", time.Since(start))
		}
		return
	}

	cb := m.circuitBreakerManager.GetCircuitBreaker(hookName)
	if err := cb.Execute(context.Background(), fn); err != nil {
		duration := time.Since(start)
		m.logger.Error("Hook execution failed with circuit breaker", 
			"hook", hookName, 
			"error", err,
			"duration", duration,
			"cb_state", cb.State().String())
	} else {
		duration := time.Since(start)
		m.logger.Debug("Hook execution completed successfully",
			"hook", hookName,
			"duration", duration,
			"cb_state", cb.State().String())
	}
}

// EmitBeforeLLM emits a before LLM call event to all registered hooks.
func (m *Manager) EmitBeforeLLM(ctx context.Context, c *LLMCallCtx) {
	if m == nil {
		return
	}
	
	var eligibleHooks []string
	for _, h := range m.hooks {
		if _, ok := h.(BeforeLLMCaller); ok {
			eligibleHooks = append(eligibleHooks, h.Info().Name)
		}
	}
	
	m.logger.Debug("Emitting BeforeLLM event",
		"eligible_hooks", len(eligibleHooks),
		"hook_names", eligibleHooks,
		"total_hooks", len(m.hooks))
	
	start := time.Now()
	for _, h := range m.hooks {
		if l, ok := h.(BeforeLLMCaller); ok {
			m.safeCall(h.Info().Name, func() error {
				l.BeforeLLMCall(ctx, c)
				return nil
			})
		}
	}
	
	m.logger.Debug("BeforeLLM event emission completed",
		"duration", time.Since(start),
		"hooks_executed", len(eligibleHooks))
}

// EmitAfterLLM emits an after LLM inference event to all registered hooks.
func (m *Manager) EmitAfterLLM(ctx context.Context, r *LLMRespCtx) {
	if m == nil {
		return
	}
	
	var eligibleHooks []string
	for _, h := range m.hooks {
		if _, ok := h.(AfterLLMInferencer); ok {
			eligibleHooks = append(eligibleHooks, h.Info().Name)
		}
	}
	
	m.logger.Debug("Emitting AfterLLM event",
		"eligible_hooks", len(eligibleHooks),
		"hook_names", eligibleHooks,
		"total_hooks", len(m.hooks))
	
	start := time.Now()
	for _, h := range m.hooks {
		if l, ok := h.(AfterLLMInferencer); ok {
			m.safeCall(h.Info().Name, func() error {
				l.AfterLLMInference(ctx, r)
				return nil
			})
		}
	}
	
	m.logger.Debug("AfterLLM event emission completed",
		"duration", time.Since(start),
		"hooks_executed", len(eligibleHooks))
}

// EmitBeforeTool emits a before tool call event to all registered hooks.
func (m *Manager) EmitBeforeTool(ctx context.Context, c *ToolCallCtx) {
	if m == nil {
		return
	}
	
	var eligibleHooks []string
	for _, h := range m.hooks {
		if _, ok := h.(BeforeToolCaller); ok {
			eligibleHooks = append(eligibleHooks, h.Info().Name)
		}
	}
	
	m.logger.Debug("Emitting BeforeTool event",
		"eligible_hooks", len(eligibleHooks),
		"hook_names", eligibleHooks,
		"tool_name", c.ToolName,
		"total_hooks", len(m.hooks))
	
	start := time.Now()
	for _, h := range m.hooks {
		if l, ok := h.(BeforeToolCaller); ok {
			m.safeCall(h.Info().Name, func() error {
				l.BeforeToolCall(ctx, c)
				return nil
			})
		}
	}
	
	m.logger.Debug("BeforeTool event emission completed",
		"duration", time.Since(start),
		"hooks_executed", len(eligibleHooks),
		"tool_name", c.ToolName)
}

// EmitAfterTool emits an after tool result event to all registered hooks.
func (m *Manager) EmitAfterTool(ctx context.Context, r *ToolResCtx) {
	if m == nil {
		return
	}
	
	var eligibleHooks []string
	for _, h := range m.hooks {
		if _, ok := h.(AfterToolResulter); ok {
			eligibleHooks = append(eligibleHooks, h.Info().Name)
		}
	}
	
	m.logger.Debug("Emitting AfterTool event",
		"eligible_hooks", len(eligibleHooks),
		"hook_names", eligibleHooks,
		"tool_name", r.ToolName,
		"total_hooks", len(m.hooks))
	
	start := time.Now()
	for _, h := range m.hooks {
		if l, ok := h.(AfterToolResulter); ok {
			m.safeCall(h.Info().Name, func() error {
				l.AfterToolResult(ctx, r)
				return nil
			})
		}
	}
	
	m.logger.Debug("AfterTool event emission completed",
		"duration", time.Since(start),
		"hooks_executed", len(eligibleHooks),
		"tool_name", r.ToolName)
}

// GetCircuitBreakerMetrics returns circuit breaker metrics for all hooks
func (m *Manager) GetCircuitBreakerMetrics() map[string]CircuitBreakerMetrics {
	if m == nil || m.circuitBreakerManager == nil {
		return make(map[string]CircuitBreakerMetrics)
	}
	
	metrics := m.circuitBreakerManager.GetAllMetrics()
	m.logger.Debug("Retrieved circuit breaker metrics",
		"hook_count", len(metrics),
		"hooks", func() []string {
			var hooks []string
			for hook := range metrics {
				hooks = append(hooks, hook)
			}
			return hooks
		}())
	
	return metrics
}

// GetHookHealthStatus returns health status for all hooks
func (m *Manager) GetHookHealthStatus() map[string]bool {
	if m == nil || m.circuitBreakerManager == nil {
		return make(map[string]bool)
	}
	
	status := m.circuitBreakerManager.GetHealthStatus()
	
	var healthy, unhealthy []string
	for hook, isHealthy := range status {
		if isHealthy {
			healthy = append(healthy, hook)
		} else {
			unhealthy = append(unhealthy, hook)
		}
	}
	
	m.logger.Debug("Retrieved hook health status",
		"total_hooks", len(status),
		"healthy_hooks", len(healthy),
		"unhealthy_hooks", len(unhealthy),
		"healthy", healthy,
		"unhealthy", unhealthy)
	
	return status
}

// ResetCircuitBreakers resets all circuit breakers
func (m *Manager) ResetCircuitBreakers() {
	if m != nil && m.circuitBreakerManager != nil {
		m.logger.Info("Resetting all circuit breakers")
		m.circuitBreakerManager.ResetAll()
		m.logger.Info("All circuit breakers reset completed")
	}
}

// ResetHookCircuitBreaker resets a specific hook's circuit breaker
func (m *Manager) ResetHookCircuitBreaker(hookName string) bool {
	if m == nil || m.circuitBreakerManager == nil {
		return false
	}
	
	m.logger.Info("Resetting circuit breaker for hook", "hook", hookName)
	success := m.circuitBreakerManager.ResetHook(hookName)
	
	if success {
		m.logger.Info("Circuit breaker reset successfully", "hook", hookName)
	} else {
		m.logger.Warn("Circuit breaker reset failed - hook not found", "hook", hookName)
	}
	
	return success
}
