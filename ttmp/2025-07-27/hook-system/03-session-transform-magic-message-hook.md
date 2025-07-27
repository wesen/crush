# Hook System v03: Session-Level TransformResult Magic Message Hook

## Executive Summary

This document designs the next iteration of the Crush hook system, implementing the TransformResult pattern from the research document to enable session-level transformation. The primary goal is to create a hook that appends a new message containing "HELLO THE MAGIC WORD IS XXX" after tool results while leaving the original tool result unmodified.

## Key Design Principles

1. **Session-Level Transformation**: Hooks can access and mutate the complete Session object
2. **TransformResult Pattern**: Use Modified, Skip, and Metadata fields for safe transformation
3. **Non-Destructive**: Original tool results remain unmodified
4. **Message Injection**: Ability to append new messages to the session
5. **Safety First**: Comprehensive error handling and isolation

## Current MVP Analysis

### Existing Hook System (v02)
The current MVP implementation provides:
- **Event-driven hooks**: BeforeLLMCaller, AfterLLMInferencer, BeforeToolCaller, AfterToolResulter
- **Simple context structs**: Basic session/agent/tool metadata
- **Observational only**: No transformation capabilities
- **Manager pattern**: Safe execution with panic recovery

### Limitations Addressed
- No access to session state or messages
- No transformation capabilities 
- Limited context information
- No ability to inject new content

## TransformResult Pattern Implementation

### Core Pattern Definition

```go
// Generic result describing an optional mutation and control-flow flags
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
```

### Session-Level Transformation Interface

```go
// TransformSessionHook enables hooks to mutate session state
type TransformSessionHook interface {
    TransformSession(ctx context.Context, session *session.Session) (TransformResult[session.Session], error)
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
    ToolCall     ToolCall      // Original tool call
    ToolResult   ToolResult    // Tool execution result
    ToolName     string        // Tool name
    Duration     time.Duration // Execution duration
    Err          error         // Tool execution error
}
```

## Enhanced Hook Interfaces

### Transformational Hook Interfaces

```go
// Hook interfaces that support transformation
type TransformLLMHook interface {
    Hook
    TransformBeforeLLM(ctx context.Context, callCtx *LLMTransformContext) (TransformResult[LLMTransformContext], error)
    TransformAfterLLM(ctx context.Context, respCtx *LLMResponseTransformContext) (TransformResult[LLMResponseTransformContext], error)
}

type TransformToolHook interface {
    Hook
    TransformBeforeTool(ctx context.Context, callCtx *ToolTransformContext) (TransformResult[ToolTransformContext], error)
    TransformAfterTool(ctx context.Context, resultCtx *ToolTransformContext) (TransformResult[ToolTransformContext], error)
}

// Session-level transformation (core capability)
type TransformSessionHook interface {
    Hook
    TransformSession(ctx context.Context, session *session.Session) (TransformResult[session.Session], error)
}
```

### Enhanced Context Structures

```go
type LLMTransformContext struct {
    *SessionHookContext
    Messages []message.Message
    Tools    []tools.ToolInfo
    Provider string
    Model    string
}

type LLMResponseTransformContext struct {
    *SessionHookContext
    Response *provider.Response
    Duration time.Duration
    Err      error
}

type ToolTransformContext struct {
    *SessionHookContext
    ToolCall   ToolCall
    ToolResult *ToolResult
    ToolName   string
    Duration   time.Duration
    Err        error
}
```

## Magic Message Hook Implementation

### Hook Implementation

```go
package main

import (
    "context"
    "fmt"
    "math/rand"
    "time"
    
    "github.com/charmbracelet/crush/internal/hooks"
    "github.com/charmbracelet/crush/internal/message"
    "github.com/charmbracelet/crush/internal/session"
)

type MagicMessageHook struct {
    messageService message.Service
    config         map[string]interface{}
    magicWords     []string
}

func (m *MagicMessageHook) Info() hooks.HookInfo {
    return hooks.HookInfo{
        Name:        "magic_message_hook",
        Version:     "1.0.0", 
        Description: "Appends magic word messages after tool results",
    }
}

func (m *MagicMessageHook) Initialize(config map[string]interface{}, services hooks.ServiceRegistry) error {
    m.config = config
    m.messageService = services.MessageService()
    
    // Default magic words
    m.magicWords = []string{"ABRACADABRA", "ALAKAZAM", "PRESTO", "HOCUS POCUS", "SESAME"}
    
    // Allow customization via config
    if words, ok := config["magic_words"].([]string); ok {
        m.magicWords = words
    }
    
    return nil
}

// Implement TransformSessionHook to get session-level access
func (m *MagicMessageHook) TransformSession(ctx context.Context, sess *session.Session) (hooks.TransformResult[session.Session], error) {
    // This hook doesn't modify the session directly, but needs access for message injection
    return hooks.NoChange[session.Session](), nil
}

// Implement AfterToolResulter for the trigger event
func (m *MagicMessageHook) AfterToolResult(ctx context.Context, resultCtx *hooks.ToolTransformContext) (hooks.TransformResult[hooks.ToolTransformContext], error) {
    // Select random magic word
    magicWord := m.magicWords[rand.Intn(len(m.magicWords))]
    magicText := fmt.Sprintf("HELLO THE MAGIC WORD IS %s", magicWord)
    
    // Create new message to inject
    _, err := m.messageService.Create(ctx, resultCtx.SessionID, message.CreateMessageParams{
        Role: message.System,
        Parts: []message.ContentPart{
            message.TextContent{Text: magicText},
        },
        Model:    "hook-system",
        Provider: "magic-message-hook",
    })
    
    if err != nil {
        return hooks.TransformResult[hooks.ToolTransformContext]{}, fmt.Errorf("failed to inject magic message: %w", err)
    }
    
    // Return original context unchanged (non-destructive)
    return hooks.NoChange[hooks.ToolTransformContext](), nil
}

// Plugin entry point
func NewHook() (hooks.Hook, error) {
    return &MagicMessageHook{}, nil
}
```

### Service Registry for Dependency Injection

```go
// ServiceRegistry provides access to core services for hooks
type ServiceRegistry interface {
    MessageService() message.Service
    SessionService() session.Service
    // Future services can be added here
}

type serviceRegistry struct {
    messageService message.Service
    sessionService session.Service
}

func NewServiceRegistry(msgSvc message.Service, sessSvc session.Service) ServiceRegistry {
    return &serviceRegistry{
        messageService: msgSvc,
        sessionService: sessSvc,
    }
}

func (sr *serviceRegistry) MessageService() message.Service {
    return sr.messageService
}

func (sr *serviceRegistry) SessionService() session.Service {
    return sr.sessionService
}
```

## Enhanced Hook Manager

### Transformation-Aware Manager

```go
package hooks

import (
    "context"
    "fmt"
    "log/slog"
    
    "github.com/charmbracelet/crush/internal/message"
    "github.com/charmbracelet/crush/internal/session"
)

type TransformManager struct {
    hooks          []Hook
    serviceRegistry ServiceRegistry
}

func NewTransformManager(serviceRegistry ServiceRegistry) *TransformManager {
    return &TransformManager{
        hooks:           make([]Hook, 0),
        serviceRegistry: serviceRegistry,
    }
}

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

// Execute transformational tool hooks with session context
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

// Safe execution with panic recovery and error handling
func (tm *TransformManager) safeTransformCall(fn func() error) error {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("Transform hook panic recovered", "panic", r)
        }
    }()
    return fn()
}

// Hook initialization interface
type HookInitializer interface {
    Initialize(config map[string]interface{}, services ServiceRegistry) error
}
```

## Integration Points

### Agent Service Integration

```go
// In internal/llm/agent/agent.go

// Update agent struct
type agent struct {
    // ... existing fields ...
    transformManager *hooks.TransformManager
    messageService   message.Service
    sessionService   session.Service
}

// Update constructor
func NewAgent(
    // ... existing params ...
    messageService message.Service,
    sessionService session.Service,
    hooksMgr *hooks.Manager,
) (Service, error) {
    // ... existing code ...
    
    // Create service registry for hooks
    serviceRegistry := hooks.NewServiceRegistry(messageService, sessionService)
    
    // Create transform manager
    transformManager := hooks.NewTransformManager(serviceRegistry)
    
    return &agent{
        // ... existing fields ...
        transformManager: transformManager,
        messageService:   messageService,
        sessionService:   sessionService,
    }, nil
}

// Enhanced after tool result hook point
func (a *agent) handleAfterToolResult(ctx context.Context, sessionID string, toolCall tools.ToolCall, result tools.ToolResult, duration time.Duration, err error) error {
    // Create enhanced context with session access
    session, sessionErr := a.sessionService.Get(ctx, sessionID)
    if sessionErr != nil {
        slog.Warn("Failed to get session for hook context", "error", sessionErr)
        session = session.Session{ID: sessionID} // Minimal fallback
    }
    
    messages, msgErr := a.messageService.List(ctx, sessionID)
    if msgErr != nil {
        slog.Warn("Failed to get messages for hook context", "error", msgErr)
        messages = []message.Message{} // Empty fallback
    }
    
    // Build rich context
    hookCtx := &hooks.ToolTransformContext{
        SessionHookContext: &hooks.SessionHookContext{
            Session:   &session,
            Messages:  messages,
            SessionID: sessionID,
            AgentID:   a.agentCfg.ID,
            Metadata:  make(map[string]any),
        },
        ToolCall:   hooks.ToolCall(toolCall),
        ToolResult: &hooks.ToolResult(result),
        ToolName:   toolCall.Name,
        Duration:   duration,
        Err:        err,
    }
    
    // Execute transformation hooks
    return a.transformManager.ExecuteTransformAfterTool(ctx, hookCtx)
}
```

### Application Integration

```go
// In internal/app/app.go

func (app *App) initializeHooks(
    messageService message.Service, 
    sessionService session.Service,
) (*hooks.TransformManager, error) {
    serviceRegistry := hooks.NewServiceRegistry(messageService, sessionService)
    manager := hooks.NewTransformManager(serviceRegistry)
    
    // Load and register hooks
    if app.config.Hooks != nil && app.config.Hooks.Enabled {
        // Plugin loading logic here
        // For now, manually register magic message hook for testing
        magicHook := &MagicMessageHook{}
        if err := manager.Add(magicHook); err != nil {
            return nil, fmt.Errorf("failed to add magic message hook: %w", err)
        }
    }
    
    return manager, nil
}
```

## Safety Mechanisms

### Error Isolation

```go
type HookExecutionError struct {
    HookName string
    Phase    string
    Err      error
}

func (hee HookExecutionError) Error() string {
    return fmt.Sprintf("hook %s failed in %s: %v", hee.HookName, hee.Phase, hee.Err)
}

// Circuit breaker for misbehaving hooks
type CircuitBreaker struct {
    failureCount    int
    failureLimit    int
    lastFailureTime time.Time
    state          string // "closed", "open", "half-open"
}

func (cb *CircuitBreaker) CanExecute() bool {
    if cb.state == "open" {
        if time.Since(cb.lastFailureTime) > 5*time.Minute {
            cb.state = "half-open"
            return true
        }
        return false
    }
    return true
}

func (cb *CircuitBreaker) RecordSuccess() {
    cb.failureCount = 0
    cb.state = "closed"
}

func (cb *CircuitBreaker) RecordFailure() {
    cb.failureCount++
    cb.lastFailureTime = time.Now()
    if cb.failureCount >= cb.failureLimit {
        cb.state = "open"
    }
}
```

### Session Mutation Safety

```go
// Session mutation validator
type SessionValidator struct{}

func (sv *SessionValidator) ValidateSessionMutation(original, modified *session.Session) error {
    // Prevent ID changes
    if original.ID != modified.ID {
        return fmt.Errorf("session ID cannot be changed")
    }
    
    // Prevent parent relationship changes
    if original.ParentSessionID != modified.ParentSessionID {
        return fmt.Errorf("parent session ID cannot be changed") 
    }
    
    // Validate token counts are non-negative
    if modified.PromptTokens < 0 || modified.CompletionTokens < 0 {
        return fmt.Errorf("token counts cannot be negative")
    }
    
    return nil
}
```

## Testing Strategy

### Unit Tests

```go
func TestMagicMessageHook_AfterToolResult(t *testing.T) {
    // Setup mock services
    mockMessageService := &mockMessageService{}
    mockSessionService := &mockSessionService{}
    serviceRegistry := hooks.NewServiceRegistry(mockMessageService, mockSessionService)
    
    // Create hook
    hook := &MagicMessageHook{}
    err := hook.Initialize(map[string]interface{}{
        "magic_words": []string{"TESTWORD"},
    }, serviceRegistry)
    require.NoError(t, err)
    
    // Create test context
    ctx := context.Background()
    resultCtx := &hooks.ToolTransformContext{
        SessionHookContext: &hooks.SessionHookContext{
            SessionID: "test-session",
            AgentID:   "test-agent",
        },
        ToolName: "test-tool",
    }
    
    // Execute hook
    result, err := hook.AfterToolResult(ctx, resultCtx)
    require.NoError(t, err)
    
    // Verify message was created
    require.True(t, mockMessageService.CreateCalled)
    require.Contains(t, mockMessageService.LastCreateParams.Parts[0].(message.TextContent).Text, "TESTWORD")
    
    // Verify original context unchanged
    require.Empty(t, result.Modified)
    require.False(t, result.Skip)
}
```

## Configuration

### Hook Configuration Example

```yaml
# crush.yaml
hooks:
  enabled: true
  transform_hooks:
    magic_message_hook:
      enabled: true
      config:
        magic_words: ["ALAKAZAM", "PRESTO", "VOILÀ"]
        trigger_tools: ["bash", "read_file"] # Optional: only trigger on specific tools
        message_role: "system" # system, assistant, user
```
