# Hook System Research for Crush Coding Agent

## Executive Summary

This document is an **RFC-style design sketch** for a hook system in the Crush coding agent.
Loadable Go plugins should be able to **intercept, observe, *and transform*** LLM calls, tool executions, session context, and their results.
All API and implementation details shown below are illustrative – they are starting points for further discussion rather than a finalized spec.

## Table of Contents

1. [Current Architecture Analysis](#current-architecture-analysis)
2. [Hook Points Identification](#hook-points-identification)
3. [Go Plugin System Overview](#go-plugin-system-overview)
4. [Hook Interface Design](#hook-interface-design)
5. [Runtime Behavior Design](#runtime-behavior-design)
6. [Integration Points](#integration-points)
7. [Security Considerations](#security-considerations)
8. [Implementation Roadmap](#implementation-roadmap)

## Current Architecture Analysis

### Core Components

The Crush agent architecture consists of several key components that interact in a coordinated manner:

#### 1. Application Orchestration (`internal/app/app.go`)
- Main application entry point that initializes all services
- Creates and manages the CoderAgent
- Sets up event subscriptions and pub/sub messaging
- Handles both interactive (TUI) and non-interactive (CLI) execution flows

#### 2. Agent Service (`internal/llm/agent/agent.go`) 
- Implements the core agent interface for running LLM conversations
- Orchestrates conversation flow: user message → LLM processing → tool calls → responses
- Manages session state, message history, and context management
- Coordinates between LLM providers and available tools

#### 3. LLM Provider System (`internal/llm/provider/provider.go`)
- Unified interface (`Provider`) with `SendMessages()` and `StreamResponse()` methods
- Multiple provider implementations: OpenAI, Anthropic, Gemini, Bedrock, Azure, VertexAI, XAI
- Event-based streaming system with types:
  - `EventContentDelta` - incremental text content
  - `EventToolUseStart/Delta/Stop` - tool call lifecycle
  - `EventThinkingDelta` - reasoning content
  - `EventComplete` - final response with usage stats

#### 4. Tool Execution System (`internal/llm/tools/tools.go`)
- Base tool interface (`BaseTool`) with `Info()`, `Name()`, and `Run()` methods
- Tool registry and factory for creating tool instances
- Tool call/response structures and execution context management
- Shell integration through persistent shell instances

#### 5. Message System (`internal/message/content.go`)
- Message content parts: TextContent, ToolCall, ToolResult, Finish, etc.
- Message lifecycle and state management
- Tool call and result tracking

### Current Data Flow

```
User Input → Agent.Run() → 
├─ LLM Provider.StreamResponse() → Events → Agent.processEvent()
├─ Tool Execution (if tool calls) → Tool.Run() → ToolResult
└─ Message Updates → Database → UI Events
```

## Hook Points Identification

Based on the architecture analysis, we identified four primary hook points where plugins can intercept operations:

### 1. LLM Call Hooks
- **Before LLM Call**: Before `provider.StreamResponse()` or `provider.SendMessages()`
- **After LLM Inference**: After LLM response is complete and `EventComplete` is received

### 2. Tool Execution Hooks  
- **Before Tool Call**: Before `tool.Run()` is executed
- **After Tool Result**: After tool execution completes and `ToolResult` is created

### Hook Trigger Locations in Code:

#### LLM Hooks:
- **File**: `internal/llm/agent/agent.go`
- **Before LLM Call**: Line 449 in `streamAndHandleEvents()` before `a.provider.StreamResponse()`
- **After LLM Inference**: Line 643 in `processEvent()` after `EventComplete` handling

#### Tool Hooks:
- **File**: `internal/llm/agent/agent.go` 
- **Before Tool Call**: Line 523 in `streamAndHandleEvents()` before `tool.Run()`
- **After Tool Result**: Line 570 in `streamAndHandleEvents()` after tool execution completes

## Go Plugin System Overview

### Plugin Architecture
Go's plugin system uses shared libraries (`.so` files on Linux, `.dylib` on macOS) that can be loaded at runtime using the `plugin` package.

### Key Limitations and Considerations:
1. **Platform Support**: Linux and macOS only (no Windows support)
2. **Build Constraints**: Plugins must be built with the same Go version and build flags as the main application
3. **Symbol Resolution**: Plugins export symbols that can be looked up at runtime
4. **Memory Management**: Plugin memory is managed by the host application
5. **Error Handling**: Plugin loading failures must be handled gracefully

### Plugin Loading Pattern:
```go
p, err := plugin.Open("path/to/plugin.so")
if err != nil {
    return err
}

symbol, err := p.Lookup("SymbolName")
if err != nil {
    return err
}

hookImpl := symbol.(HookInterface)
```

## Hook Interface Design

### Observational vs Transformational Hooks *(sketch)*

The initial design focused on read-only *observer* hooks.  
To unlock more advanced use-cases we now allow **transformational hooks** that may
inspect *and modify* the data flowing through the agent:

* **Transform Scope** – hooks may access or mutate:
  - Individual `Message`, `ToolCall`, or `ToolResult`
  - The complete `Session` (title, summary, metadata, tokens, etc.)
  - Arbitrary key/value metadata on `HookContext`
  - **Conversation context slices** (e.g. trim message history for context window management)

* **Session-Level Transformation** – every hook may optionally implement a session transformer to mutate the overarching context:

```pseudo
# Lets a hook rewrite anything about the session (messages, stats, metadata)
TransformSessionHook {
  TransformSession(ctx, s *Session) (TransformResult[Session], error)
}

# Example: after a tool call completes, drop messages tagged as temporary scratch
AfterToolResult(ctx, resCtx) {
  if resCtx.ToolName == "bash" {
     tmp := filter(resCtx.Session.Messages, m => !m.Meta.TempScratch)
     return TransformResult{ Modified: &Session{Messages: tmp} }
  }
  return TransformResult{}
}
```

All hooks **receive the current `Session` object** in their context, so even hooks that only expose `BeforeLLMCall` can still choose to mutate the session through the returned `TransformResult`.

* **Illustrative API idea** (details open for discussion):

```go
// Generic result describing an optional mutation and control-flow flags.
type TransformResult[T any] struct {
    // Replace original value with Modified if set.
    Modified *T
    // If Skip=true the underlying operation is bypassed.
    Skip bool
    // Additional metadata forwarded to subsequent hooks.
    Metadata map[string]any
}

// Example transformational LLM hook.
type TransformLLMHook interface {
    BeforeLLMCall(ctx context.Context, callCtx LLMCallContext) (TransformResult[LLMCallContext], error)
    AfterLLMInference(ctx context.Context, respCtx LLMResponseContext) (TransformResult[LLMResponseContext], error)
}

// Example session-level hook (sketch).
type SessionHook interface {
    BeforeSession(ctx context.Context, s *session.Session) error
    AfterSession(ctx context.Context, s *session.Session, err error) error
}
```

These APIs are **sketches** – concrete generics, pointer semantics, or alternative
patterns (e.g. functional options) are deliberately left flexible so that the
final implementation can choose the most ergonomic approach.

### Core Hook Interfaces (pseudocode)

```pseudo
# Context passed to every hook
HookContext {
  SessionID string
  MessageID string
  AgentID   string
  Metadata  map[string]any
}

# Hook categories
LLMHook       { BeforeLLMCall, AfterLLMInference }
ToolHook      { BeforeToolCall, AfterToolResult }
SessionHook   { BeforeSession, AfterSession }

# Transform support (generic sketch)
TransformResult<T> {
  Modified *T      # optional replacement value
  Skip     bool    # if true, underlying operation is skipped
  Metadata map[string]any
}
```

Fields & generics are intentionally high-level; exact types can evolve during implementation.

### Hook Error Handling
Hooks should never crash the agent. Implementations MUST capture errors and log
them without stopping the main flow.  A lightweight structure is enough:

```pseudo
HookError { HookName, Phase, Err }
```

Aggregation / circuit-breaker logic is TBD.

## Runtime Behavior Design

The runtime layer will revolve around **three cooperating components** (sketch):

1. **Hook Registry** – concurrency-safe map keyed by hook name with `Load`, `Unload`, and `List` operations.
2. **Hook Manager** – discovers `*.so` files in configured directories, loads them via the registry, applies per-hook configuration.
3. **Hook Executor** – orchestrates execution, applies timeouts, aggregates errors, and enforces circuit-breaker rules.

```pseudo
HookRegistry.Load(path, cfg)   -> error
HookManager.LoadAll(dirs)     -> error
HookExecutor.Run(phase, ctx, data) -> error
```

Exact locking strategy, plugin signature verification, and telemetry hooks are
implementation details to be fleshed out during coding.

## Integration Points

### Agent Service Integration

The main integration points in the agent service (`internal/llm/agent/agent.go`):

```go
// Add to agent struct
type agent struct {
    // ... existing fields ...
    hookManager *hooks.HookManager
}

// Update NewAgent function
func NewAgent(...) (Service, error) {
    // ... existing code ...
    
    // Initialize hook manager
    hookConfig := cfg.Hooks
    hookManager := hooks.NewHookManager(hookConfig.HookDirs, hookConfig.Hooks)
    if hookConfig.Enabled {
        if err := hookManager.LoadAllHooks(); err != nil {
            slog.Warn("Failed to load some hooks", "error", err)
        }
    }
    
    return &agent{
        // ... existing fields ...
        hookManager: hookManager,
    }, nil
}

// Update streamAndHandleEvents method
func (a *agent) streamAndHandleEvents(ctx context.Context, sessionID string, msgHistory []message.Message) (message.Message, *message.Message, error) {
    // ... existing code up to line 448 ...
    
    // HOOK POINT: Before LLM Call
    if a.hookManager != nil {
        llmCtx := &hooks.LLMCallContext{
            HookContext: hooks.HookContext{
                SessionID: sessionID,
                MessageID: assistantMsg.ID,
                AgentID:   a.agentCfg.ID,
            },
            Messages: msgHistory,
            Tools:    slices.Collect(a.tools.Seq()),
            Provider: a.providerID,
            Model:    a.Model().ID,
        }
        a.hookManager.ExecuteLLMHooks(ctx, "before_llm_call", llmCtx)
    }
    
    startTime := time.Now()
    eventChan := a.provider.StreamResponse(ctx, msgHistory, slices.Collect(a.tools.Seq()))
    
    // ... existing event processing code ...
    
    // HOOK POINT: After LLM Inference (in processEvent for EventComplete)
    if event.Type == provider.EventComplete && a.hookManager != nil {
        responseCtx := &hooks.LLMResponseContext{
            HookContext: hooks.HookContext{
                SessionID: sessionID,
                MessageID: assistantMsg.ID,
                AgentID:   a.agentCfg.ID,
            },
            Response: event.Response,
            Duration: time.Since(startTime),
        }
        a.hookManager.ExecuteLLMHooks(ctx, "after_llm_inference", responseCtx)
    }
    
    // ... tool execution loop around line 482 ...
    
    for i, toolCall := range toolCalls {
        // ... existing code ...
        
        // HOOK POINT: Before Tool Call
        if a.hookManager != nil {
            toolCtx := &hooks.ToolCallContext{
                HookContext: hooks.HookContext{
                    SessionID: sessionID,
                    MessageID: assistantMsg.ID,
                    AgentID:   a.agentCfg.ID,
                },
                ToolCall: tools.ToolCall{
                    ID:    toolCall.ID,
                    Name:  toolCall.Name,
                    Input: toolCall.Input,
                },
                ToolName: toolCall.Name,
            }
            a.hookManager.ExecuteToolHooks(ctx, "before_tool_call", toolCtx)
        }
        
        toolStartTime := time.Now()
        
        // ... existing tool execution code ...
        
        // HOOK POINT: After Tool Result
        if a.hookManager != nil {
            resultCtx := &hooks.ToolResultContext{
                HookContext: hooks.HookContext{
                    SessionID: sessionID,
                    MessageID: assistantMsg.ID,
                    AgentID:   a.agentCfg.ID,
                },
                ToolCall: tools.ToolCall{
                    ID:    toolCall.ID,
                    Name:  toolCall.Name,
                    Input: toolCall.Input,
                },
                Result:   toolResponse,
                Duration: time.Since(toolStartTime),
                Error:    toolErr,
            }
            a.hookManager.ExecuteToolHooks(ctx, "after_tool_result", resultCtx)
        }
    }
    
    // ... rest of existing code ...
}
```

### Application Integration

Update the main application (`internal/app/app.go`) to support hooks configuration:

```go
// Add to App struct
type App struct {
    // ... existing fields ...
    HookManager *hooks.HookManager
}

// Update New function
func New(ctx context.Context, conn *sql.DB, cfg *config.Config) (*App, error) {
    // ... existing code ...
    
    // Initialize hooks if enabled
    var hookManager *hooks.HookManager
    if cfg.Hooks != nil && cfg.Hooks.Enabled {
        hookManager = hooks.NewHookManager(cfg.Hooks.HookDirs, cfg.Hooks.Hooks)
        if err := hookManager.LoadAllHooks(); err != nil {
            slog.Warn("Failed to load some hooks", "error", err)
        }
    }
    
    app := &App{
        // ... existing fields ...
        HookManager: hookManager,
    }
    
    // ... rest of existing code ...
}

// Update Shutdown method
func (app *App) Shutdown() {
    // ... existing shutdown code ...
    
    // Shutdown hooks
    if app.HookManager != nil {
        app.HookManager.ShutdownAll()
    }
}
```

### Configuration Integration

Update the configuration system to support hooks (`internal/config/config.go`):

```go
type Config struct {
    // ... existing fields ...
    Hooks *HookConfig `yaml:"hooks,omitempty"`
}

type HookConfig struct {
    Enabled   bool                           `yaml:"enabled"`
    HookDirs  []string                      `yaml:"hook_dirs"`
    Hooks     map[string]map[string]interface{} `yaml:"hooks"`
}
```

## Security Considerations

### Plugin Security
1. **Sandboxing**: Consider using syscall filtering or containers for plugin isolation
2. **Verification**: Implement plugin signature verification before loading
3. **Resource Limits**: Set memory and CPU limits for plugin execution
4. **Permission System**: Integrate with existing permission system for sensitive operations

### Error Handling
1. **Graceful Degradation**: Hook failures should not break main agent functionality
2. **Timeout Protection**: Set timeouts for hook execution to prevent blocking
3. **Circuit Breaker**: Disable misbehaving hooks automatically
4. **Audit Logging**: Log all hook executions for security monitoring

### Example Security Implementation:
```go
type SecureHookExecutor struct {
    *HookExecutor
    timeout        time.Duration
    circuitBreaker map[string]*CircuitBreaker
}

func (she *SecureHookExecutor) executeHookWithSecurity(ctx context.Context, hook Hook, phase string, data interface{}) error {
    // Check circuit breaker
    if she.circuitBreaker[hook.Info().Name].IsOpen() {
        return fmt.Errorf("hook %s circuit breaker is open", hook.Info().Name)
    }
    
    // Set timeout
    timeoutCtx, cancel := context.WithTimeout(ctx, she.timeout)
    defer cancel()
    
    // Execute with panic recovery
    defer func() {
        if r := recover(); r != nil {
            she.logger.Error("Hook panic recovered", "hook", hook.Info().Name, "panic", r)
            she.circuitBreaker[hook.Info().Name].RecordFailure()
        }
    }()
    
    // Execute hook
    return she.executeHook(timeoutCtx, hook, phase, data)
}
```

## Implementation Roadmap

### Phase 1: Core Infrastructure
1. **Hook Interface Definition** (1-2 days)
   - Define core hook interfaces
   - Create hook context structures
   - Implement basic hook registry

2. **Plugin Loading System** (2-3 days)
   - Implement plugin discovery and loading
   - Create hook manager with configuration support
   - Add error handling and logging

3. **Agent Integration** (2-3 days)
   - Add hook points to agent service
   - Integrate hook execution in LLM and tool flows
   - Update configuration system

### Phase 2: Security and Reliability
1. **Security Measures** (3-4 days)
   - Implement timeout protection
   - Add circuit breaker pattern
   - Create audit logging system

2. **Error Handling** (1-2 days)
   - Graceful degradation on hook failures
   - Hook execution isolation
   - Recovery mechanisms

## Example Hook Implementation

### Logging Hook Plugin

```go
// File: examples/logging_hook/main.go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "time"
    
    "github.com/charmbracelet/crush/internal/hooks"
)

type LoggingHook struct {
    logFile   *os.File
    logger    *log.Logger
    logLevel  string
}

func (lh *LoggingHook) Info() hooks.HookInfo {
    return hooks.HookInfo{
        Name:        "logging_hook",
        Version:     "1.0.0",
        Description: "Logs all LLM calls and tool executions",
        Author:      "Crush Team",
        SupportedEvents: []string{
            "before_llm_call",
            "after_llm_inference", 
            "before_tool_call",
            "after_tool_result",
        },
    }
}

func (lh *LoggingHook) Initialize(config map[string]interface{}) error {
    logFile, ok := config["log_file"].(string)
    if !ok {
        logFile = "/tmp/crush-hooks.log"
    }
    
    file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        return fmt.Errorf("failed to open log file: %w", err)
    }
    
    lh.logFile = file
    lh.logger = log.New(file, "[HOOK] ", log.LstdFlags)
    lh.logLevel = config["log_level"].(string)
    
    return nil
}

func (lh *LoggingHook) Shutdown() error {
    if lh.logFile != nil {
        return lh.logFile.Close()
    }
    return nil
}

func (lh *LoggingHook) BeforeLLMCall(ctx context.Context, callCtx *hooks.LLMCallContext) error {
    data, _ := json.Marshal(map[string]interface{}{
        "event":     "before_llm_call",
        "timestamp": time.Now().Unix(),
        "session_id": callCtx.SessionID,
        "model":     callCtx.Model,
        "provider":  callCtx.Provider,
        "num_messages": len(callCtx.Messages),
        "num_tools": len(callCtx.Tools),
    })
    lh.logger.Printf("LLM_CALL_START: %s", string(data))
    return nil
}

func (lh *LoggingHook) AfterLLMInference(ctx context.Context, responseCtx *hooks.LLMResponseContext) error {
    data, _ := json.Marshal(map[string]interface{}{
        "event":      "after_llm_inference",
        "timestamp":  time.Now().Unix(),
        "session_id": responseCtx.SessionID,
        "duration_ms": responseCtx.Duration.Milliseconds(),
        "input_tokens": responseCtx.Response.Usage.InputTokens,
        "output_tokens": responseCtx.Response.Usage.OutputTokens,
        "finish_reason": responseCtx.Response.FinishReason,
        "error": responseCtx.Error != nil,
    })
    lh.logger.Printf("LLM_INFERENCE_COMPLETE: %s", string(data))
    return nil
}

func (lh *LoggingHook) BeforeToolCall(ctx context.Context, callCtx *hooks.ToolCallContext) error {
    data, _ := json.Marshal(map[string]interface{}{
        "event":      "before_tool_call",
        "timestamp":  time.Now().Unix(),
        "session_id": callCtx.SessionID,
        "tool_name":  callCtx.ToolName,
        "tool_call_id": callCtx.ToolCall.ID,
    })
    lh.logger.Printf("TOOL_CALL_START: %s", string(data))
    return nil
}

func (lh *LoggingHook) AfterToolResult(ctx context.Context, resultCtx *hooks.ToolResultContext) error {
    data, _ := json.Marshal(map[string]interface{}{
        "event":       "after_tool_result",
        "timestamp":   time.Now().Unix(),
        "session_id":  resultCtx.SessionID,
        "tool_name":   resultCtx.ToolCall.Name,
        "tool_call_id": resultCtx.ToolCall.ID,
        "duration_ms": resultCtx.Duration.Milliseconds(),
        "is_error":    resultCtx.Result.IsError,
        "error":       resultCtx.Error != nil,
    })
    lh.logger.Printf("TOOL_RESULT_COMPLETE: %s", string(data))
    return nil
}

// NewHook is the plugin entry point
func NewHook() (hooks.Hook, error) {
    return &LoggingHook{}, nil
}

func main() {
    // This file needs to be built as a plugin:
    // go build -buildmode=plugin -o logging_hook.so main.go
}
```

### Building the Plugin:
```bash
cd examples/logging_hook
go build -buildmode=plugin -o logging_hook.so main.go
```

### Configuration Example:
```yaml
# crush.yaml
hooks:
  enabled: true
  hook_dirs:
    - "./hooks"
    - "~/.crush/hooks"
  hooks:
    logging_hook:
      enabled: true
      config:
        log_file: "/var/log/crush-hooks.log"
        log_level: "info"
```

## Conclusion

This research provides a comprehensive foundation for implementing a plugin-based hook system in the Crush coding agent. The design focuses on:

1. **Extensibility**: Clean interfaces that allow third-party development
2. **Performance**: Minimal overhead for the main agent operations
3. **Security**: Proper isolation and error handling for plugin execution
4. **Maintainability**: Clear separation of concerns and well-defined APIs

The hook system will enable powerful extensibility for monitoring, logging, metrics collection, request/response transformation, and custom integrations while maintaining the reliability and performance of the core agent system.

