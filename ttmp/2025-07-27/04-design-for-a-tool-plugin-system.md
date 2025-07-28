# Tool Plugin System Design Sketch for Crush Coding Agent

## Purpose & Scope

Enable third-party developers to drop in **Go plugins** (`*.so`) that add _new tools_ to Crush without modifying the core codebase.  Each plugin supplies one or more **Tool implementations** that can:

1. Be invoked by the LLM via standard tool-call semantics (`name`, `arguments` JSON)
2. Access the **full `Session` context** (message history, metadata, permissions, etc.)
3. **Return both** a `ToolResponse` _and_ an **optional mutated `Session`** allowing tools to reshape conversation state (e.g. summarise history, add system messages, update metadata).

This design mirrors the recently updated Hook system (§01) but targets _tool execution_ rather than pre/post interception.

> 🔍 **RFC status** – All interfaces are illustrative; final shapes will stabilise through prototyping & review.

## Table of Contents

1. High-Level Architecture
1.5. Current Tool Architecture Analysis
2. Core Concepts & Data Structures
3. Plugin API (pseudocode)
4. Runtime Loading & Registration
5. Invocation Flow
6. Security & Isolation
7. Example Plugin Scenario
8. Open Questions
9. Integration with Current Systems

---

## 1. High-Level Architecture

```
+-------------+        plugin load       +-----------------+
| ToolManager | ----------------------> | .so Tool Plugin |
+-------------+                         +-----------------+
         | register (ToolInfo)                  |
         v                                       |
Crush Agent -- calls --> ToolExecutor -- invokes --> Tool.Run()
```

1. **ToolManager** scans configured directories, loads plugins, and registers `Tool` instances.
2. **Agent** derives _callable_ tool list from ToolManager when prompting the LLM.
3. When an LLM tool-call appears, **ToolExecutor** retrieves the corresponding Tool and executes it with `(args, session)`.
4. Result propagates back to Agent → Message Service → UI.

## 1.5. Current Tool Architecture Analysis

### Built-in Tools (`internal/llm/tools/`)

Crush currently has **10+ built-in tools** implementing the `BaseTool` interface:

```go
type BaseTool interface {
    Info() ToolInfo        // name, description, JSON schema
    Name() string          // unique identifier
    Run(ctx, ToolCall) (ToolResponse, error)
}
```

**Examples:** `bash`, `edit`, `write`, `view`, `grep`, `glob`, `ls`, `fetch`, `download`, `sourcegraph`, `diagnostics`.

### MCP Tool Integration (`internal/llm/agent/mcp-tools.go`)

**Model Context Protocol (MCP)** tools are loaded dynamically from external processes:

```pseudo
# MCP tool discovery flow
Config.MCP[name] -> MCPClient.Initialize() -> ListTools() -> wrap each as mcpTool{}

# Runtime execution
mcpTool.Run() -> MCPClient.CallTool() -> parse response -> ToolResponse
```

MCP supports **3 transport types**: stdio, HTTP, SSE. Each MCP tool gets prefixed as `mcp_{name}_{tool}`.

### Agent Tool Registry (`internal/llm/agent/agent.go:173`)

The agent assembles tools via a **lazy slice** that combines:

```go
allTools := []tools.BaseTool{
    // Built-in tools
    tools.NewBashTool(permissions, cwd),
    tools.NewEditTool(lspClients, permissions, history, cwd),
    // ... 8 more built-ins
}

// Dynamic additions
mcpTools := GetMCPTools(ctx, permissions, cfg)
allTools = append(allTools, mcpTools...)

if agentTool != nil {  // recursive agent tool
    allTools = append(allTools, agentTool)
}

// Filter by allowlist if configured
if agentCfg.AllowedTools != nil { /* filter */ }
```

### Tool Execution Flow (`internal/llm/agent/agent.go:497`)

When the LLM generates tool calls:

```pseudo
1. Agent.streamAndHandleEvents() detects tool_calls in LLM response
2. For each ToolCall:
   - Find tool by name in agent.tools.Seq()
   - Emit "before tool" hook
   - tool.Run(ctx, ToolCall{ID, Name, Input}) in goroutine
   - Emit "after tool" hook  
   - Create ToolResult message
3. Continue conversation with tool results
```

**Key gaps** for plugin system:
- No session context passed to `tool.Run()`
- No mechanism for tools to return modified session state
- Static tool registration at agent creation time

## 2. Core Concepts & Data Structures

```pseudo
ToolInfo {
  Name        string   # unique identifier, exposed to LLM
  Description string   # human-readable (for JSON schema)
  Parameters  JSONSchema   # argument schema
}

ToolCall {
  ID     string
  Name   string
  Input  json.RawMessage   # as sent by LLM
}

ToolResponse {
  Content  string | bytes | structured
  IsError  bool
  Metadata map[string]any
}

TransformResult<T> {
  Modified *T   # optional replacement (e.g. Session)
  Skip     bool # bypass core flow (rare for tools)
  Meta     map[string]any
}
```

### Differences from Current `BaseTool`

The proposed plugin interface extends current tools in **2 key ways**:

1. **Session Context Access**: `Run(ctx, call, *Session)` vs current `Run(ctx, call)`
2. **Transform Capability**: `(ToolResponse, TransformResult[Session], error)` vs `(ToolResponse, error)`

This allows plugin tools to:
- Read full conversation history, metadata, token counts
- Modify session state (trim messages, add system prompts, update metadata)
- Still return normal tool responses for the LLM to process

## 3. Plugin API (pseudocode)

```pseudo
# Entry point symbol exported by plugin
func NewTools() ([]Tool, error)

# Minimal interface every plugin tool must satisfy
Tool {
  Info() ToolInfo
  Run(ctx, call ToolCall, session *Session) (ToolResponse, TransformResult[Session], error)
}

# Optional extended interface for streaming output (future work)
StreamingTool {
  Tool
  Stream(ctx, call, session) -> chan ToolChunk
}
```

### Key Points

* **Session is passed by pointer** so in-place edits are possible _but_ returning a `TransformResult` makes intent explicit and allows ToolExecutor to mediate concurrent mutations.
* `ToolResponse` remains the primary user-visible output; modified session is secondary.
* Multiple tools may live in one plugin file – `NewTools()` returns a slice.

### Backwards Compatibility

Plugin tools can **opt into** session access:

```pseudo
# Legacy compatibility wrapper
type LegacyPluginTool interface {
  Info() ToolInfo  
  Run(ctx, call ToolCall) (ToolResponse, error)  # no session
}

# Adapter pattern
func WrapLegacyTool(legacy LegacyPluginTool) Tool {
  return legacyAdapter{legacy}
}

func (la legacyAdapter) Run(ctx, call, session) (ToolResponse, TransformResult[Session], error) {
  resp, err := la.legacy.Run(ctx, call)
  return resp, TransformResult[Session]{}, err  # no session changes
}
```

## 4. Runtime Loading & Registration

```pseudo
ToolRegistry.Load(path string) error  # locate NewTools symbol, iterate slice
ToolManager.LoadAll(dirs []string)
ToolManager.List() []ToolInfo         # surfaced to Agent for JSON schema
```

Same registry / manager / executor triad used by hooks.  Concurrency control & lazy initialisation follow the same patterns.

### Integration with Current Tool Discovery

```pseudo
# Enhanced agent toolFn (agent.go:173)
allTools := []BaseTool{
    # ... existing built-in tools
}

# NEW: Plugin tool discovery
pluginTools := ToolManager.LoadAll(cfg.PluginDirs)
allTools = append(allTools, pluginTools...)

# Existing MCP integration unchanged
mcpTools := GetMCPTools(ctx, permissions, cfg)
allTools = append(allTools, mcpTools...)

# Tool name conflict resolution (TBD)
# - Plugin tools override built-ins?
# - Namespace prefixing like MCP: "plugin_{name}_{tool}"?
```

## 5. Invocation Flow

```pseudo
Agent detects LLM tool_call -> ToolExecutor.Execute(call)
    sessionCopy := deepCopy(Session)
    resp, sessDelta, err := tool.Run(ctx, call, sessionCopy)
    if sessDelta.Modified != nil {
        merge(Session, sessDelta.Modified)
        MessageService.Publish(updated Session)
    }
    createToolResultMessage(resp)
```

* Tools operate on a copy to avoid partial mutations on failure.
* `merge` strategy (replace vs patch) is TBD – likely strategic per field.
* Any session change triggers pub/sub so UI updates instantaneously.

### Enhanced Agent Execution Loop

Current flow (simplified):

```pseudo
# Current: agent.go:497
for toolCall := range toolCalls {
    tool := findToolByName(toolCall.Name)
    resp, err := tool.Run(ctx, toolCall)  # no session context
    toolResults[i] = message.ToolResult{ToolCallID, Content, IsError}
}
createToolMessage(toolResults)
```

**Enhanced flow** for plugin tools:

```pseudo
for toolCall := range toolCalls {
    tool := findToolByName(toolCall.Name)
    
    if pluginTool, ok := tool.(PluginTool); ok {
        sessionCopy := session.DeepCopy()
        resp, transform, err := pluginTool.Run(ctx, toolCall, sessionCopy)
        
        # Apply session mutations
        if transform.Modified != nil {
            session.Merge(transform.Modified)
            MessageService.Publish(UpdatedEvent, session)
        }
        
        toolResults[i] = resp.toToolResult()
    } else {
        # Legacy tool path unchanged
        resp, err := tool.Run(ctx, toolCall)
        toolResults[i] = resp.toToolResult()
    }
}
```

## 7. Example Plugin Scenario – "AutoSummarise"

Goal: after each tool execution, trim the transcript and append a summary message.

```pseudo
struct AutoSummariseTool implements Tool {
  Info() -> {Name:"autosummarise", Description:"Summarise chat history", Parameters:{max_tokens:int}}

  Run(ctx, call, session) {
     max := call.Input.max_tokens
     summary := summarise(session.Messages, max)
     session.Messages = keepLast(session.Messages, 10) + [summaryMessage(summary)]
     return ToolResponse{Content:"ok"}, TransformResult{Modified:&session}, nil
  }
}
```

The plugin removes older messages and inserts a summary – demonstrating full session mutation.

## 9. Integration with Current Systems

### Phase 1: Extend Current Architecture

```pseudo
# Add plugin support to existing tool flow
type PluginTool interface {
    BaseTool  # inherit current interface
    RunWithSession(ctx, call, session) (ToolResponse, TransformResult[Session], error)
}

# Enhanced agent.go tool execution
if pluginTool, ok := tool.(PluginTool); ok {
    # New enhanced flow
} else {
    # Existing flow unchanged
}
```

### Phase 2: Unified Tool Interface 

Longer-term: **migrate all tools** (built-in, MCP, plugin) to session-aware interface:

```pseudo
# Unified interface evolution
type Tool interface {
    Info() ToolInfo
    Run(ctx, call, session) (ToolResponse, TransformResult[Session], error)
}

# Built-in tools get session access
bash.Run(ctx, call, session) {
    # Could use session.WorkingDir, session.Env, etc.
}

# MCP tools get adapter
mcpTool.Run(ctx, call, session) {
    resp := mcpClient.CallTool(call)
    return resp, TransformResult{}, nil  # most MCP tools won't use session
}
```

### Configuration Schema

```yaml
# crush.yaml extensions
tool_plugins:
  enabled: true
  plugin_dirs:
    - "./plugins"
    - "~/.crush/plugins"  
  tools:
    autosummarise:
      enabled: true
      config:
        max_messages: 50
        summary_prompt: "Summarise this conversation"
```

---

### Next Steps (non-binding)

1. Prototype Registry & Manager (1-2 days)
2. Build sample plugins (`hello_world`, `autosummarise`)
3. Integrate permission prompts & timeout enforcement
4. Draft developer documentation & skeleton CI for plugin compilation
5. **Decision**: extend current `BaseTool` vs create parallel `PluginTool` hierarchy
6. **Prototype**: session deep-copy and merge strategies
7. **Design**: tool name conflict resolution (namespacing vs override)

The proposed system brings first-class, session-aware tools to Crush, aligning with the flexible, transformative capabilities outlined in the Hook RFC while keeping implementation surface small and ergonomic for Go developers. 