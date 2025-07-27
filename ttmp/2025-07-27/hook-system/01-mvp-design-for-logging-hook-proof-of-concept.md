# MVP Design for Logging Hook Proof of Concept

## Goal
Create the simplest possible hook system that can demonstrate a logging hook observing:
- LLM calls (before/after)  
- Tool executions (before/after)

**Success criteria**: Start Crush agent, run a single prompt, and see a LoggingHook print structured logs for each event.

## Essential Components (Must Ship)

### 1. Minimal Hook API (Read-Only)
**File**: `internal/hooks/hooks.go`

```go
// Core hook metadata
type HookInfo struct {
    Name, Version, Description string
}

// Event-specific marker interfaces (opt-in per hook)
type BeforeLLMCaller     interface{ BeforeLLMCall(ctx context.Context, c *LLMCallCtx) }
type AfterLLMInferencer  interface{ AfterLLMInference(ctx context.Context, r *LLMRespCtx) }
type BeforeToolCaller    interface{ BeforeToolCall(ctx context.Context, c *ToolCallCtx) }
type AfterToolResulter   interface{ AfterToolResult(ctx context.Context, r *ToolResCtx) }

// Generic hook interface (all hooks satisfy this)
type Hook interface {
    Info() HookInfo
}

// Minimal event context structs (only what logger needs)
type LLMCallCtx  struct{ SessionID, AgentID, Model string }
type LLMRespCtx  struct{ SessionID, AgentID string; Duration time.Duration }
type ToolCallCtx struct{ SessionID, AgentID, ToolName string }
type ToolResCtx  struct{ SessionID, AgentID, ToolName string; Duration time.Duration; Err error }
```

### 2. Lightweight Manager
**File**: `internal/hooks/manager.go`

```go
type Manager struct{ hooks []Hook }

func (m *Manager) Add(h Hook) { m.hooks = append(m.hooks, h) }

// Event emitters with panic recovery
func (m *Manager) EmitBeforeLLM(ctx context.Context, c *LLMCallCtx) {
    for _, h := range m.hooks {
        if l, ok := h.(BeforeLLMCaller); ok { 
            safeCall(func() { l.BeforeLLMCall(ctx, c) })
        }
    }
}
// Similar for EmitAfterLLM, EmitBeforeTool, EmitAfterTool

func safeCall(fn func()) {
    defer func() {
        if r := recover(); r != nil {
            slog.Warn("Hook panic recovered", "panic", r)
        }
    }()
    fn()
}
```

### 3. Agent Integration
**File**: `internal/llm/agent/agent.go`

Minimal changes:
- Add `hooks *hooks.Manager` to agent struct
- Accept optional Manager in NewAgent()
- Insert 4 hook emit calls at the right locations:

```go
// Before LLM call
m.EmitBeforeLLM(ctx, &hooks.LLMCallCtx{
    SessionID: sessionID, 
    AgentID: a.agentCfg.ID, 
    Model: a.Model().ID,
})

// After LLM inference (in EventComplete handler)
m.EmitAfterLLM(ctx, &hooks.LLMRespCtx{
    SessionID: sessionID,
    AgentID: a.agentCfg.ID,
    Duration: time.Since(startTime),
})

// Before/after tool calls in tool execution loop
```

### 4. Built-in Logging Hook
**File**: `internal/hooks/builtin/logging/logging.go`

```go
type LoggingHook struct { logger *slog.Logger }

func (l *LoggingHook) Info() hooks.HookInfo {
    return hooks.HookInfo{
        Name: "logging_hook",
        Version: "1.0.0", 
        Description: "Logs LLM calls and tool executions",
    }
}

func (l *LoggingHook) BeforeLLMCall(ctx context.Context, c *hooks.LLMCallCtx) {
    l.logger.Info("before_llm_call", slog.Any("context", c))
}
// Implement remaining 3 methods
```

### 5. Application Wiring
Enable via environment variable in app initialization:

```go
mgr := &hooks.Manager{}
if os.Getenv("CRUSH_USE_LOG_HOOK") == "1" {
    mgr.Add(logging.New(slog.Default()))
}
agent := llm.NewAgent(..., mgr)
```

## What We Explicitly Defer

❌ Go plugin loading (plugin.Open, directory scanning)  
❌ YAML/CLI configuration system changes  
❌ TransformResult, session mutation, generics  
❌ Circuit breakers, timeouts, sandboxing  
❌ Hook unload/shutdown callbacks  
❌ Security hardening  
❌ Tests beyond manual verification  

## Implementation Steps

1. **Create hook system** (`internal/hooks/` - ~150 LoC)
   - Core interfaces and context structs
   - Manager with event emitters and panic recovery

2. **Add logging hook** (`internal/hooks/builtin/logging/` - ~80 LoC)
   - Implement all 4 event methods
   - Structured logging with slog

3. **Integrate with agent** (~15-20 LoC changes)
   - Thread Manager through app and agent
   - Add 4 emit calls at hook points

4. **Manual test**:
   ```bash
   CRUSH_USE_LOG_HOOK=1 go run ./cmd/crush
   ```
   Issue a prompt that triggers LLM + tool usage, observe logs:
   ```
   "before_llm_call ..."
   "after_llm_inference ..." 
   "before_tool_call ..."
   "after_tool_result ..."
   ```

## Success Criteria
- Hook system compiles and runs without errors
- Logging hook captures all 4 event types during normal agent operation
- No impact on agent performance or reliability
- Clean foundation for future plugin system expansion

This MVP demonstrates the core observability pattern and provides a stable base for incrementally adding the full plugin system features described in the research document.
