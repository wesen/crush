package hooks

import (
	"context"
	"time"
)

// HookInfo contains metadata about a hook.
type HookInfo struct {
	Name        string
	Version     string
	Description string
}

// Hook is the base interface that all hooks must implement.
type Hook interface {
	Info() HookInfo
}

// Event-specific marker interfaces (hooks opt-in by implementing these)

// BeforeLLMCaller is implemented by hooks that want to observe LLM calls before execution.
type BeforeLLMCaller interface {
	BeforeLLMCall(ctx context.Context, c *LLMCallCtx)
}

// AfterLLMInferencer is implemented by hooks that want to observe LLM responses after completion.
type AfterLLMInferencer interface {
	AfterLLMInference(ctx context.Context, r *LLMRespCtx)
}

// BeforeToolCaller is implemented by hooks that want to observe tool calls before execution.
type BeforeToolCaller interface {
	BeforeToolCall(ctx context.Context, c *ToolCallCtx)
}

// AfterToolResulter is implemented by hooks that want to observe tool results after execution.
type AfterToolResulter interface {
	AfterToolResult(ctx context.Context, r *ToolResCtx)
}

// Event context structs

// LLMCallCtx contains context for LLM call events.
type LLMCallCtx struct {
	SessionID string
	AgentID   string
	Model     string
}

// LLMRespCtx contains context for LLM response events.
type LLMRespCtx struct {
	SessionID string
	AgentID   string
	Duration  time.Duration
}

// ToolCallCtx contains context for tool call events.
type ToolCallCtx struct {
	SessionID string
	AgentID   string
	ToolName  string
}

// ToolResCtx contains context for tool result events.
type ToolResCtx struct {
	SessionID string
	AgentID   string
	ToolName  string
	Duration  time.Duration
	Err       error
}
