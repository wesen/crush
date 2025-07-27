package hooks

import (
	"context"
	"fmt"
	"log/slog"
)

// TransformManager manages transformation-capable hooks
type TransformManager struct {
	hooks           []Hook
	serviceRegistry ServiceRegistry
}

// NewTransformManager creates a new transform manager
func NewTransformManager(serviceRegistry ServiceRegistry) *TransformManager {
	return &TransformManager{
		hooks:           make([]Hook, 0),
		serviceRegistry: serviceRegistry,
	}
}

// Add adds a hook to the manager and initializes it if needed
func (tm *TransformManager) Add(h Hook) error {
	// Initialize hook with service registry if it supports it
	if initializer, ok := h.(HookInitializer); ok {
		if err := initializer.Initialize(nil, tm.serviceRegistry); err != nil {
			return fmt.Errorf("failed to initialize hook %s: %w", h.Info().Name, err)
		}
	}
	tm.hooks = append(tm.hooks, h)
	return nil
}

// ExecuteTransformAfterTool executes transformational tool hooks with session context
func (tm *TransformManager) ExecuteTransformAfterTool(ctx context.Context, resultCtx *ToolTransformContext) error {
	for _, h := range tm.hooks {
		if transformHook, ok := h.(TransformToolHook); ok {
			if err := tm.safeTransformCall(func() error {
				result, err := transformHook.TransformAfterTool(ctx, resultCtx)
				if err != nil {
					return err
				}

				// Apply transformation result
				if result.Modified != nil {
					*resultCtx = *result.Modified
				}

				// Handle skip logic (for future use)
				if result.Skip {
					slog.Debug("Tool result skipped by hook", "hook", h.Info().Name)
				}

				// Merge metadata
				if len(result.Metadata) > 0 {
					if resultCtx.Metadata == nil {
						resultCtx.Metadata = make(map[string]any)
					}
					for k, v := range result.Metadata {
						resultCtx.Metadata[k] = v
					}
				}

				return nil
			}); err != nil {
				slog.Error("Transform hook failed",
					"hook", h.Info().Name,
					"phase", "after_tool_result",
					"error", err)
			}
		}
	}
	return nil
}

// safeTransformCall executes a function with panic recovery
func (tm *TransformManager) safeTransformCall(fn func() error) error {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Transform hook panic recovered", "panic", r)
		}
	}()
	return fn()
}
