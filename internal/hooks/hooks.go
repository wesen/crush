package hooks

import (
	"context"
	"time"

	"github.com/charmbracelet/crush/internal/llm/tools"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
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

// TransformResult describes an optional mutation and control-flow flags
type TransformResult[T any] struct {
	// Replace original value with Modified if set
	Modified *T `json:"modified,omitempty"`
	// If Skip=true the underlying operation is bypassed
	Skip bool `json:"skip,omitempty"`
	// Additional metadata forwarded to subsequent hooks
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Convenience constructors
func Modified[T any](value T) TransformResult[T] {
	return TransformResult[T]{Modified: &value}
}

func Skip[T any]() TransformResult[T] {
	return TransformResult[T]{Skip: true}
}

func NoChange[T any]() TransformResult[T] {
	return TransformResult[T]{}
}

func WithMetadata[T any](key string, value any) TransformResult[T] {
	return TransformResult[T]{Metadata: map[string]any{key: value}}
}

// ServiceRegistry provides access to core services for hooks
type ServiceRegistry interface {
	MessageService() message.Service
	SessionService() session.Service
}

// Enhanced context structures with session access
type SessionHookContext struct {
	Session   *session.Session  // Complete session object
	Messages  []message.Message // Current message history
	SessionID string            // Session identifier
	AgentID   string            // Agent identifier
	Metadata  map[string]any    // Hook chain metadata
}

// Tool-specific context with session transformation capability
type ToolTransformContext struct {
	*SessionHookContext
	ToolCall   tools.ToolCall      // Original tool call
	ToolResult *tools.ToolResponse // Tool execution result
	ToolName   string              // Tool name
	Duration   time.Duration       // Execution duration
	Err        error               // Tool execution error
}

// HookInitializer interface for hooks that need initialization
type HookInitializer interface {
	Initialize(config map[string]interface{}, services ServiceRegistry) error
}

// Transform hook interfaces

// TransformToolHook enables hooks to mutate tool execution context
type TransformToolHook interface {
	Hook
	TransformAfterTool(ctx context.Context, resultCtx *ToolTransformContext) (TransformResult[ToolTransformContext], error)
}

// TransformSessionHook enables hooks to mutate session state
type TransformSessionHook interface {
	Hook
	TransformSession(ctx context.Context, session *session.Session) (TransformResult[session.Session], error)
}
