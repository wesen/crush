package hooks

import (
	"context"
	"log/slog"
)

// Manager manages a collection of hooks and emits events to them.
type Manager struct {
	hooks []Hook
}

// New creates a new hook manager.
func New() *Manager {
	return &Manager{
		hooks: make([]Hook, 0),
	}
}

// Add adds a hook to the manager.
func (m *Manager) Add(h Hook) {
	if m == nil {
		return
	}
	m.hooks = append(m.hooks, h)
}

// safeCall executes a function with panic recovery.
func safeCall(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("Hook panic recovered", "panic", r)
		}
	}()
	fn()
}

// EmitBeforeLLM emits a before LLM call event to all registered hooks.
func (m *Manager) EmitBeforeLLM(ctx context.Context, c *LLMCallCtx) {
	if m == nil {
		return
	}
	for _, h := range m.hooks {
		if l, ok := h.(BeforeLLMCaller); ok {
			safeCall(func() { l.BeforeLLMCall(ctx, c) })
		}
	}
}

// EmitAfterLLM emits an after LLM inference event to all registered hooks.
func (m *Manager) EmitAfterLLM(ctx context.Context, r *LLMRespCtx) {
	if m == nil {
		return
	}
	for _, h := range m.hooks {
		if l, ok := h.(AfterLLMInferencer); ok {
			safeCall(func() { l.AfterLLMInference(ctx, r) })
		}
	}
}

// EmitBeforeTool emits a before tool call event to all registered hooks.
func (m *Manager) EmitBeforeTool(ctx context.Context, c *ToolCallCtx) {
	if m == nil {
		return
	}
	for _, h := range m.hooks {
		if l, ok := h.(BeforeToolCaller); ok {
			safeCall(func() { l.BeforeToolCall(ctx, c) })
		}
	}
}

// EmitAfterTool emits an after tool result event to all registered hooks.
func (m *Manager) EmitAfterTool(ctx context.Context, r *ToolResCtx) {
	if m == nil {
		return
	}
	for _, h := range m.hooks {
		if l, ok := h.(AfterToolResulter); ok {
			safeCall(func() { l.AfterToolResult(ctx, r) })
		}
	}
}
