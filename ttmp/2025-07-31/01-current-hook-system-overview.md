# Crush Hook System - Current Implementation Analysis

## Executive Summary

After thorough analysis of the hook system implementation, I've identified several critical issues that prevent the system from working correctly. While the architecture is well-designed, there are significant gaps in the integration between components that render the system non-functional.

## Architecture Overview

### Core Components

1. **Hook Interfaces** (`internal/hooks/hooks.go`)
   - Base `Hook` interface with `Info()` method
   - Event-specific interfaces: `BeforeLLMCaller`, `AfterLLMInferencer`, `BeforeToolCaller`, `AfterToolResulter`
   - Transform interfaces: `TransformToolHook`, `TransformSessionHook`
   - Service registry interface for hook initialization

2. **Event Manager** (`internal/hooks/manager.go`)
   - Manages observational hooks (before/after LLM calls, tool calls)
   - Circuit breaker integration for fault tolerance
   - Event emission methods: `EmitBeforeLLM`, `EmitAfterLLM`, `EmitBeforeTool`, `EmitAfterTool`

3. **Transform Manager** (`internal/hooks/transform_manager.go`)
   - Manages transformational hooks that can modify session/tool state
   - `ExecuteTransformAfterTool` method for tool result transformation
   - Circuit breaker protection for transform operations

4. **Plugin System** (`internal/hooks/plugin_manager.go`, `plugin_loader.go`)
   - Dynamic plugin loading from `.so` files
   - Plugin lifecycle management
   - Configuration-driven plugin loading

5. **Built-in Hooks**
   - **Logging Hook** (`internal/hooks/builtin/logging/logging.go`): Structured JSON logging
   - **Magic Message Hook** (`internal/hooks/builtin/magicmessage/magicmessage.go`): Session transformation example

## Critical Issues Identified

### 1. **Missing Service Registry Implementation** ❌

**Issue**: The `NewServiceRegistry` function is called in `agent.go:221` but the function doesn't exist in the hooks package.

**Location**: `internal/llm/agent/agent.go:221`
```go
serviceRegistry := hooks.NewServiceRegistry(messages, sessions)
```

**Problem**: This function is not defined in the hooks package, causing compilation errors.

**Fix Required**: The `NewServiceRegistry` function needs to be implemented in `internal/hooks/registry.go`.

### 2. **Incomplete Agent Integration** ❌

**Issue**: The agent constructor receives only the event manager (`hooksMgr`) but not the transform manager.

**Location**: `internal/llm/agent/agent.go:93-102`
```go
func NewAgent(
    // ... other params ...
    hooksMgr *hooks.Manager,  // Only event manager passed
) (Service, error)
```

**Problem**: The agent creates its own transform manager internally, but the app-level transform manager (with circuit breaker config) is not passed through.

**Impact**: Circuit breaker configuration from app config is not applied to the agent's transform manager.

### 3. **Missing Event Emission Points** ❌

**Issue**: While the agent has hook emission calls, they're incomplete and inconsistent.

**Current Emission Points**:
- ✅ `EmitBeforeLLM` - Called in `streamAndHandleEvents` (line 464)
- ✅ `EmitBeforeTool` - Called before tool execution (line 530)
- ✅ `EmitAfterTool` - Called after tool execution (line 562)
- ❌ `EmitAfterLLM` - Called in `processEvent` but only for completion events

**Problem**: The `EmitAfterLLM` is only called for completion events, not for all LLM inference results.

### 4. **Transform Hook Registration Issues** ❌

**Issue**: Transform hooks are registered with the agent after agent creation, but the agent's internal transform manager is created without proper configuration.

**Location**: `internal/app/app.go:330-340`
```go
// Register loaded plugin hooks with agent
for _, hook := range pluginMgr.GetLoadedHooks() {
    if err := app.CoderAgent.AddTransformHook(hook); err != nil {
        slog.Error("Failed to add plugin transform hook", "hook", hook.Info().Name, "err", err)
    }
}
```

**Problem**: The agent's transform manager is created with default circuit breaker config, not the app-level configuration.

### 5. **Plugin Loading Configuration Issues** ❌

**Issue**: Plugin configuration structure doesn't match the expected format.

**Location**: `internal/app/app.go:310-320`
```go
pluginConfigs[name] = map[string]interface{}{
    "path":     config.Path,
    "config":   config.Config,
    "disabled": config.Disabled,
}
```

**Problem**: The plugin loader expects different configuration structure than what's being passed.

## Component Integration Analysis

### App Initialization Flow

1. **App Setup** (`internal/app/app.go:272-350`)
   - Creates hook managers with circuit breaker config
   - Sets up plugin manager
   - Loads plugins from configuration
   - Creates agent with event manager only

2. **Agent Creation** (`internal/llm/agent/agent.go:93-240`)
   - Creates its own transform manager with default config
   - Receives event manager from app
   - Creates service registry (missing implementation)

3. **Hook Registration** (`internal/app/app.go:330-350`)
   - Registers plugin hooks with agent after creation
   - Registers built-in hooks conditionally

### Event Flow

1. **LLM Call Flow**:
   ```
   Agent.Run() → streamAndHandleEvents() → EmitBeforeLLM() → LLM Call → processEvent() → EmitAfterLLM()
   ```

2. **Tool Execution Flow**:
   ```
   Tool Call → EmitBeforeTool() → Tool Execution → EmitAfterTool() → handleTransformAfterTool()
   ```

### Transform Flow

1. **Tool Transform**:
   ```
   Tool Result → handleTransformAfterTool() → transformManager.ExecuteTransformAfterTool()
   ```

## Missing Implementations

### 1. Service Registry Implementation

**File**: `internal/hooks/registry.go`
**Missing**: `NewServiceRegistry` function

**Required Implementation**:
```go
func NewServiceRegistry(msgSvc message.Service, sessSvc session.Service) ServiceRegistry {
    return &serviceRegistry{
        messageService: msgSvc,
        sessionService: sessSvc,
    }
}
```

### 2. Agent Constructor Update

**File**: `internal/llm/agent/agent.go`
**Missing**: Transform manager parameter

**Required Change**:
```go
func NewAgent(
    // ... existing params ...
    hooksMgr *hooks.Manager,
    transformMgr *hooks.TransformManager,  // Add this parameter
) (Service, error)
```

### 3. App Integration Update

**File**: `internal/app/app.go`
**Missing**: Pass transform manager to agent

**Required Change**:
```go
app.CoderAgent, err = agent.NewAgent(
    // ... existing params ...
    hooksMgr,
    transformMgr,  // Pass the configured transform manager
)
```

## Configuration Issues

### 1. Circuit Breaker Configuration

**Current**: Agent creates transform manager with default config
**Required**: Use app-level circuit breaker configuration

### 2. Plugin Configuration Structure

**Current**: App passes nested config structure
**Required**: Flatten configuration or update plugin loader expectations

## Performance and Reliability

### Circuit Breaker System ✅

The circuit breaker implementation is solid:
- Three-state operation (Closed/Open/Half-Open)
- Configurable thresholds and timeouts
- Per-hook configuration support
- Automatic recovery mechanisms

### Plugin Loading System ✅

The plugin system is well-implemented:
- Dynamic `.so` file loading
- Error handling and graceful degradation
- Plugin lifecycle management
- Configuration-driven loading

### Built-in Hooks ✅

The built-in hooks are properly implemented:
- Logging hook with structured JSON output
- Magic message hook with session transformation
- Environment variable controls

## Recommendations for Fixes

### Priority 1: Critical Fixes

1. **Implement Service Registry** (`internal/hooks/registry.go`)
   - Add `NewServiceRegistry` function
   - Implement `ServiceRegistry` interface

2. **Update Agent Constructor** (`internal/llm/agent/agent.go`)
   - Add transform manager parameter
   - Remove internal transform manager creation
   - Use passed transform manager

3. **Update App Integration** (`internal/app/app.go`)
   - Pass configured transform manager to agent
   - Ensure circuit breaker config is applied

### Priority 2: Integration Fixes

1. **Fix Plugin Configuration** (`internal/app/app.go`)
   - Align plugin config structure with loader expectations
   - Test plugin loading end-to-end

2. **Complete Event Emission** (`internal/llm/agent/agent.go`)
   - Ensure all LLM events emit hooks
   - Add missing emission points

### Priority 3: Testing and Validation

1. **Integration Tests**
   - Test complete hook execution flow
   - Verify circuit breaker behavior
   - Test plugin loading and execution

2. **End-to-End Tests**
   - Test with real LLM calls and tool execution
   - Verify hook output and transformations

## Conclusion

The hook system architecture is well-designed and the individual components are solid, but there are critical integration gaps that prevent the system from working. The main issues are:

1. **Missing service registry implementation**
2. **Incomplete agent integration** (transform manager not passed through)
3. **Configuration mismatches** between app and agent levels

Once these critical fixes are implemented, the system should work as designed with proper circuit breaker protection, plugin loading, and both observational and transformational hooks.

**Status**: ❌ **NOT FUNCTIONAL** - Requires critical fixes before use
**Estimated Fix Time**: 2-3 hours for critical issues
**Testing Required**: Comprehensive integration testing after fixes 