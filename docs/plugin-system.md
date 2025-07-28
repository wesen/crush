# Crush Plugin System

The Crush plugin system allows extending functionality through Go plugins (.so files) that implement hook interfaces.

## Overview

The plugin system supports two types of hooks:

1. **Event Hooks** - Observe events without modification (logging, metrics, etc.)
2. **Transform Hooks** - Intercept and modify execution flow (message injection, tool result modification, etc.)

## Plugin Development

### Basic Plugin Structure

A plugin must export a `NewHook()` function that returns a `hooks.Hook`:

```go
package main

import "github.com/charmbracelet/crush/hooks"

type MyHook struct {
    // plugin state
}

// Required: Entry point function
func NewHook() hooks.Hook {
    return &MyHook{}
}

// Required: Hook metadata
func (m *MyHook) Info() hooks.HookInfo {
    return hooks.HookInfo{
        Name:        "my_hook",
        Version:     "1.0.0", 
        Description: "My custom hook",
    }
}

// Optional: Initialization with config and services
func (m *MyHook) Initialize(config map[string]interface{}, services hooks.ServiceRegistry) error {
    // Initialize plugin with configuration and access to services
    return nil
}
```

### Hook Types

#### Event Hooks

Event hooks observe without modifying:

```go
// Observe LLM calls before execution
func (m *MyHook) BeforeLLMCall(ctx context.Context, c *hooks.LLMCallCtx) {
    // Log, track, or observe LLM calls
}

// Observe tool calls before execution  
func (m *MyHook) BeforeToolCall(ctx context.Context, c *hooks.ToolCallCtx) {
    // Log, track, or observe tool calls
}

// Observe LLM responses after completion
func (m *MyHook) AfterLLMInference(ctx context.Context, r *hooks.LLMRespCtx) {
    // Log, track, or observe LLM responses
}

// Observe tool results after execution
func (m *MyHook) AfterToolResult(ctx context.Context, r *hooks.ToolResCtx) {
    // Log, track, or observe tool results
}
```

#### Transform Hooks

Transform hooks can modify execution:

```go
// Modify tool execution context and inject messages
func (m *MyHook) TransformAfterTool(ctx context.Context, resultCtx *hooks.ToolTransformContext) (hooks.TransformResult[hooks.ToolTransformContext], error) {
    // Access session context
    sessionID := resultCtx.SessionID
    toolName := resultCtx.ToolName
    
    // Inject a message
    _, err := m.messageService.Create(ctx, sessionID, hooks.CreateMessageParams{
        Role: hooks.Assistant,
        Parts: []hooks.ContentPart{
            hooks.TextContent{Text: "Hook was here!"},
        },
        Model:    "hook-system",
        Provider: "my-hook",
    })
    
    if err != nil {
        return hooks.TransformResult[hooks.ToolTransformContext]{}, err
    }
    
    // Return unchanged context
    return hooks.NoChange[hooks.ToolTransformContext](), nil
}

// Modify session state
func (m *MyHook) TransformSession(ctx context.Context, session *hooks.Session) (hooks.TransformResult[hooks.Session], error) {
    // Modify session if needed
    return hooks.NoChange[hooks.Session](), nil
}
```

### Transform Results

Transform hooks can return different result types:

```go
// No changes
return hooks.NoChange[T](), nil

// Modify the value
return hooks.Modified(newValue), nil

// Skip the operation
return hooks.Skip[T](), nil

// Add metadata for next hooks
return hooks.WithMetadata[T]("key", "value"), nil
```

### Building Plugins

Plugins must be built as shared libraries:

```bash
go build -buildmode=plugin -o myplugin.so main.go
```

### Plugin Dependencies

Plugins should use the public `github.com/charmbracelet/crush/hooks` package for types and interfaces.

## Configuration

### Plugin Directories

Configure directories to scan for plugins:

```json
{
  "hooks": {
    "plugin_dirs": [
      "./plugins",
      "/usr/local/lib/crush/plugins"
    ]
  }
}
```

### Individual Plugins

Configure specific plugins with custom settings:

```json
{
  "hooks": {
    "plugins": {
      "my_hook": {
        "path": "./plugins/my_hook.so",
        "config": {
          "setting1": "value1",
          "setting2": 42
        },
        "disabled": false
      }
    }
  }
}
```

### Combined Configuration

```json
{
  "hooks": {
    "plugin_dirs": ["./plugins"],
    "plugins": {
      "special_hook": {
        "path": "/custom/path/special.so",
        "config": {
          "api_key": "${SPECIAL_API_KEY}"
        }
      }
    }
  }
}
```

## Error Handling

The plugin system handles errors gracefully:

- Plugin loading errors are logged but don't crash the system
- Failed plugins are tracked and reported
- Hook execution errors are caught and logged
- Panic recovery prevents hook failures from crashing Crush

## Services Available to Plugins

Plugins can access core services through the `ServiceRegistry`:

```go
func (m *MyHook) Initialize(config map[string]interface{}, services hooks.ServiceRegistry) error {
    // Access message service for creating/managing messages
    m.messageService = services.MessageService()
    
    // Access session service for session management
    m.sessionService = services.SessionService()
    
    return nil
}
```

## Examples

See `examples/test_hook/` for a complete working example.

## Best Practices

1. **Error Handling**: Always handle errors gracefully
2. **Logging**: Use structured logging for debugging
3. **Performance**: Keep hook execution fast to avoid slowing down operations
4. **State Management**: Be careful with shared state in concurrent environments
5. **Configuration**: Use the config parameter for customizable behavior
6. **Testing**: Test plugins thoroughly before deployment

## Security Considerations

- Plugins run in the same process as Crush
- Validate all input and configuration
- Be careful with external network calls
- Follow Go security best practices
- Consider using the permissions system for sensitive operations

## Debugging

Enable debug logging to see plugin loading and execution:

```bash
export CRUSH_DEBUG=1
```

Check plugin status and errors in the logs.
