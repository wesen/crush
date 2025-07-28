package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// TransformManager manages transformation-capable hooks
type TransformManager struct {
	hooks                 []Hook
	serviceRegistry       ServiceRegistry
	circuitBreakerManager *CircuitBreakerManager
	logger                *slog.Logger
}

// NewTransformManager creates a new transform manager
func NewTransformManager(serviceRegistry ServiceRegistry) *TransformManager {
	logger := slog.Default().With("component", "transform_manager")
	logger.Debug("Creating new transform manager")
	return &TransformManager{
		hooks:                 make([]Hook, 0),
		serviceRegistry:       serviceRegistry,
		circuitBreakerManager: NewCircuitBreakerManager(DefaultCircuitBreakerConfig()),
		logger:                logger,
	}
}

// NewTransformManagerWithCircuitBreakerConfig creates a new transform manager with custom circuit breaker configuration
func NewTransformManagerWithCircuitBreakerConfig(serviceRegistry ServiceRegistry, defaultConfig CircuitBreakerConfig, perHookConfig map[string]CircuitBreakerConfig) *TransformManager {
	logger := slog.Default().With("component", "transform_manager")
	logger.Debug("Creating new transform manager with custom circuit breaker config",
		"default_config", defaultConfig,
		"per_hook_configs", len(perHookConfig))
	
	cbManager := NewCircuitBreakerManager(defaultConfig)
	for hookName, config := range perHookConfig {
		logger.Debug("Setting per-hook circuit breaker config",
			"hook", hookName,
			"config", config)
		cbManager.SetHookConfig(hookName, config)
	}
	return &TransformManager{
		hooks:                 make([]Hook, 0),
		serviceRegistry:       serviceRegistry,
		circuitBreakerManager: cbManager,
		logger:                logger,
	}
}

// Add adds a hook to the manager and initializes it if needed
func (tm *TransformManager) Add(h Hook) error {
	start := time.Now()
	info := h.Info()
	
	tm.logger.Debug("Adding hook to transform manager",
		"hook", info.Name,
		"version", info.Version,
		"description", info.Description,
		"current_hooks", len(tm.hooks))

	// Initialize hook with service registry if it supports it
	if initializer, ok := h.(HookInitializer); ok {
		initStart := time.Now()
		tm.logger.Debug("Initializing transform hook", "hook", info.Name)
		
		if err := initializer.Initialize(nil, tm.serviceRegistry); err != nil {
			tm.logger.Error("Failed to initialize transform hook",
				"hook", info.Name,
				"error", err,
				"init_duration", time.Since(initStart))
			return fmt.Errorf("failed to initialize hook %s: %w", h.Info().Name, err)
		}
		
		tm.logger.Debug("Transform hook initialized successfully",
			"hook", info.Name,
			"init_duration", time.Since(initStart))
	}
	
	tm.hooks = append(tm.hooks, h)
	
	totalDuration := time.Since(start)
	tm.logger.Info("Hook added to transform manager successfully",
		"hook", info.Name,
		"version", info.Version,
		"total_hooks", len(tm.hooks),
		"add_duration", totalDuration)
	
	return nil
}

// ExecuteTransformAfterTool executes transformational tool hooks with session context
func (tm *TransformManager) ExecuteTransformAfterTool(ctx context.Context, resultCtx *ToolTransformContext) error {
	start := time.Now()
	
	var eligibleHooks []string
	for _, h := range tm.hooks {
		if _, ok := h.(TransformToolHook); ok {
			eligibleHooks = append(eligibleHooks, h.Info().Name)
		}
	}
	
	tm.logger.Debug("Starting TransformAfterTool execution",
		"eligible_hooks", len(eligibleHooks),
		"hook_names", eligibleHooks,
		"total_hooks", len(tm.hooks),
		"tool_name", resultCtx.ToolName)
	
	var transformsApplied int
	var transformErrors []string
	
	for _, h := range tm.hooks {
		if transformHook, ok := h.(TransformToolHook); ok {
			hookStart := time.Now()
			hookName := h.Info().Name
			
			tm.logger.Debug("Executing transform hook",
				"hook", hookName,
				"tool_name", resultCtx.ToolName)
			
			if err := tm.safeTransformCall(hookName, func() error {
				result, err := transformHook.TransformAfterTool(ctx, resultCtx)
				if err != nil {
					return err
				}

				// Apply transformation result
				if result.Modified != nil {
					tm.logger.Debug("Applying transformation result",
						"hook", hookName,
						"tool_name", resultCtx.ToolName,
						"has_modifications", true)
					*resultCtx = *result.Modified
					transformsApplied++
				}

				// Handle skip logic (for future use)
				if result.Skip {
					tm.logger.Debug("Tool result skipped by transform hook",
						"hook", hookName,
						"tool_name", resultCtx.ToolName)
				}

				// Merge metadata
				if len(result.Metadata) > 0 {
					tm.logger.Debug("Merging transformation metadata",
						"hook", hookName,
						"metadata_keys", len(result.Metadata))
					
					if resultCtx.Metadata == nil {
						resultCtx.Metadata = make(map[string]any)
					}
					for k, v := range result.Metadata {
						resultCtx.Metadata[k] = v
					}
				}

				return nil
			}); err != nil {
				hookDuration := time.Since(hookStart)
				tm.logger.Error("Transform hook execution failed",
					"hook", hookName,
					"phase", "after_tool_result",
					"tool_name", resultCtx.ToolName,
					"error", err,
					"hook_duration", hookDuration)
				transformErrors = append(transformErrors, fmt.Sprintf("%s: %v", hookName, err))
			} else {
				hookDuration := time.Since(hookStart)
				tm.logger.Debug("Transform hook executed successfully",
					"hook", hookName,
					"tool_name", resultCtx.ToolName,
					"hook_duration", hookDuration)
			}
		}
	}
	
	totalDuration := time.Since(start)
	tm.logger.Debug("TransformAfterTool execution completed",
		"total_duration", totalDuration,
		"eligible_hooks", len(eligibleHooks),
		"transforms_applied", transformsApplied,
		"errors", len(transformErrors),
		"tool_name", resultCtx.ToolName)
	
	if len(transformErrors) > 0 {
		tm.logger.Warn("Some transform hooks failed",
			"failed_count", len(transformErrors),
			"errors", transformErrors,
			"tool_name", resultCtx.ToolName)
	}
	
	return nil
}

// safeTransformCall executes a function with panic recovery and circuit breaker protection
func (tm *TransformManager) safeTransformCall(hookName string, fn func() error) error {
	start := time.Now()
	
	tm.logger.Debug("Executing transform hook with protection", "hook", hookName)
	
	if tm.circuitBreakerManager == nil {
		// Fallback to old behavior if no circuit breaker manager
		defer func() {
			duration := time.Since(start)
			if r := recover(); r != nil {
				tm.logger.Error("Transform hook panic recovered",
					"hook", hookName,
					"panic", r,
					"duration", duration)
			} else {
				tm.logger.Debug("Transform hook execution completed (no circuit breaker)",
					"hook", hookName,
					"duration", duration)
			}
		}()
		
		err := fn()
		if err != nil {
			tm.logger.Warn("Transform hook execution failed",
				"hook", hookName,
				"error", err,
				"duration", time.Since(start))
		}
		return err
	}

	cb := tm.circuitBreakerManager.GetCircuitBreaker(hookName)
	err := cb.Execute(context.Background(), fn)
	
	duration := time.Since(start)
	if err != nil {
		tm.logger.Error("Transform hook execution failed with circuit breaker",
			"hook", hookName,
			"error", err,
			"duration", duration,
			"cb_state", cb.State().String())
	} else {
		tm.logger.Debug("Transform hook execution completed successfully",
			"hook", hookName,
			"duration", duration,
			"cb_state", cb.State().String())
	}
	
	return err
}

// GetCircuitBreakerMetrics returns circuit breaker metrics for transform hooks
func (tm *TransformManager) GetCircuitBreakerMetrics() map[string]CircuitBreakerMetrics {
	if tm == nil || tm.circuitBreakerManager == nil {
		return make(map[string]CircuitBreakerMetrics)
	}
	
	metrics := tm.circuitBreakerManager.GetAllMetrics()
	tm.logger.Debug("Retrieved transform hook circuit breaker metrics",
		"hook_count", len(metrics))
	
	return metrics
}

// GetHookHealthStatus returns health status for transform hooks
func (tm *TransformManager) GetHookHealthStatus() map[string]bool {
	if tm == nil || tm.circuitBreakerManager == nil {
		return make(map[string]bool)
	}
	
	status := tm.circuitBreakerManager.GetHealthStatus()
	
	var healthy, unhealthy []string
	for hook, isHealthy := range status {
		if isHealthy {
			healthy = append(healthy, hook)
		} else {
			unhealthy = append(unhealthy, hook)
		}
	}
	
	tm.logger.Debug("Retrieved transform hook health status",
		"total_hooks", len(status),
		"healthy_hooks", len(healthy),
		"unhealthy_hooks", len(unhealthy),
		"healthy", healthy,
		"unhealthy", unhealthy)
	
	return status
}

// ResetCircuitBreakers resets all circuit breakers for transform hooks
func (tm *TransformManager) ResetCircuitBreakers() {
	if tm != nil && tm.circuitBreakerManager != nil {
		tm.logger.Info("Resetting all transform hook circuit breakers")
		tm.circuitBreakerManager.ResetAll()
		tm.logger.Info("All transform hook circuit breakers reset completed")
	}
}

// ResetHookCircuitBreaker resets a specific transform hook's circuit breaker
func (tm *TransformManager) ResetHookCircuitBreaker(hookName string) bool {
	if tm == nil || tm.circuitBreakerManager == nil {
		return false
	}
	
	tm.logger.Info("Resetting circuit breaker for transform hook", "hook", hookName)
	success := tm.circuitBreakerManager.ResetHook(hookName)
	
	if success {
		tm.logger.Info("Transform hook circuit breaker reset successfully", "hook", hookName)
	} else {
		tm.logger.Warn("Transform hook circuit breaker reset failed - hook not found", "hook", hookName)
	}
	
	return success
}

// GetLoadedHooks returns all loaded transform hooks
func (tm *TransformManager) GetLoadedHooks() []Hook {
	if tm == nil {
		return nil
	}
	
	result := make([]Hook, len(tm.hooks))
	copy(result, tm.hooks)
	
	var hookNames []string
	for _, h := range tm.hooks {
		hookNames = append(hookNames, h.Info().Name)
	}
	
	tm.logger.Debug("Retrieved loaded transform hooks",
		"hook_count", len(result),
		"hook_names", hookNames)
	
	return result
}
