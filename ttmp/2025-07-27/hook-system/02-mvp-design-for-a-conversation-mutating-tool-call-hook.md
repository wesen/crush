# Design: Conversation-Mutating Hook System (Content Transformation MVP)

**Status**: Design Phase  
**Date**: 2025-07-27  
**Goal**: Build on the logging hook MVP to create hooks that can safely mutate conversation content for testing LLM behavior.

## Current State Analysis

### MVP Hook System Architecture

Based on analysis of the current implementation:

**Hook Interfaces** (`internal/hooks/hooks.go`):
- `BeforeLLMCaller` - observes LLM calls before execution
- `AfterLLMInferencer` - observes LLM responses after completion
- `BeforeToolCaller` - observes tool calls before execution  
- `AfterToolResulter` - observes tool results after execution

**Manager** (`internal/hooks/manager.go`):
- Safe execution with panic recovery via `safeCall()`
- Opt-in interface pattern for hook capabilities
- Emit methods for each event type

**Integration Points** (`internal/llm/agent/agent.go`):
- **Line 456-462**: BeforeLLM emission before provider.StreamResponse()
- **Line 682-689**: AfterLLM emission after EventComplete processing
- **Line 538-545**: BeforeTool emission before tool execution
- **Line 555-564**: AfterTool emission after tool execution with duration/error

**Current Limitations**:
- Hooks are **read-only observers** - cannot modify content
- Event context structs contain only metadata, no mutable content
- No mechanism for content transformation or injection

## Design Goals

Create a **"Magic Word" hook** that:

1. **Appends** "HELLO THE MAGIC WORD IS XXX" to tool call results or LLM responses
2. **Tests** that the LLM correctly interprets this magic word in subsequent calls
3. **Validates** the conversation flow continues normally with injected content
4. **Provides foundation** for content mutation capabilities

## New Hook Interfaces Design

### Content Mutation Interfaces

Adopt the **`TransformResult[T]`** pattern used across the wider RFCs.  Hooks now
return a `TransformResult` describing any mutation instead of replacing the
context struct directly.

```go
// TransformContentHook allows hooks to-transform streaming/LLM content.
type TransformContentHook interface {
    MutateContent(ctx context.Context, c *ContentMutationCtx) (TransformResult[*ContentMutationCtx], error)
}

// TransformToolResultHook allows hooks to transform tool results.
type TransformToolResultHook interface {
    MutateToolResult(ctx context.Context, r *ToolResultMutationCtx) (TransformResult[*ToolResultMutationCtx], error)
}
```

`TransformResult[T]` (defined in the shared RFC) has the shape:

```go
type TransformResult[T any] struct {
    Modified *T            // Optional replacement value
    Skip     bool          // If true, downstream processing is skipped
    Metadata map[string]any
}
```

Returning `nil` or `Modified == nil` means **no change**.  The hook manager will
take care of chaining results.

### New Event Context Structs

```go
// ContentMutationCtx contains content that can be safely modified
type ContentMutationCtx struct {
	SessionID string
	AgentID   string
	Content   string           // Mutable content
	Source    ContentSource    // Where this content originated
	Metadata  map[string]any   // Additional context
}

// ToolResultMutationCtx contains tool result content that can be modified
type ToolResultMutationCtx struct {
	SessionID  string
	AgentID    string
	ToolName   string
	ToolCallID string
	Content    string           // Mutable tool result content
	IsError    bool
	Metadata   map[string]any   // Additional context
}

// ContentSource indicates where content originated
type ContentSource string

const (
	ContentSourceLLMResponse ContentSource = "llm_response"
	ContentSourceToolResult  ContentSource = "tool_result"
	ContentSourceUserInput   ContentSource = "user_input"
)
```

## Hook Manager Extensions

### New Emission Methods

```go
// EmitContentMutation applies TransformContentHook chain, returning final state.
func (m *Manager) EmitContentMutation(ctx context.Context, c *ContentMutationCtx) *ContentMutationCtx {
    if m == nil {
        return c
    }
    cur := c
    for _, h := range m.hooks {
        if tr, ok := h.(TransformContentHook); ok {
            safeCall(func() {
                res, err := tr.MutateContent(ctx, cur)
                if err == nil && res.Modified != nil {
                    cur = res.Modified
                }
            })
        }
    }
    return cur
}

// EmitToolResultMutation applies TransformToolResultHook chain.
func (m *Manager) EmitToolResultMutation(ctx context.Context, r *ToolResultMutationCtx) *ToolResultMutationCtx {
    if m == nil {
        return r
    }
    cur := r
    for _, h := range m.hooks {
        if tr, ok := h.(TransformToolResultHook); ok {
            safeCall(func() {
                res, err := tr.MutateToolResult(ctx, cur)
                if err == nil && res.Modified != nil {
                    cur = res.Modified
                }
            })
        }
    }
    return cur
}
```

### Safe Mutation Guidelines

1. **Immutable Input**: Original context structs are never modified
2. **Chain of Responsibility**: Each hook receives the result from the previous hook
3. **Panic Recovery**: Content mutations wrapped in `safeCall()` 
4. **Nil Safety**: Returns original content if hook panics or returns nil

## Magic Word Hook Implementation

### Core Hook Structure

```go
package magicword

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"log/slog"

	"github.com/charmbracelet/crush/internal/hooks"
)

// MagicWordHook injects magic words into conversation content for testing
type MagicWordHook struct {
	logger          *slog.Logger
	currentMagicWord string
	wordBank        []string
	enabled         bool
}

// New creates a new magic word hook
func New(logger *slog.Logger) *MagicWordHook {
	return &MagicWordHook{
		logger: logger,
		wordBank: []string{
			"ELEPHANT", "RAINBOW", "QUANTUM", "BUTTERFLY", "THUNDER",
			"CRYSTAL", "PHOENIX", "MOUNTAIN", "OCEAN", "GALAXY",
		},
		enabled: os.Getenv("CRUSH_USE_MAGIC_WORD_HOOK") == "1",
	}
}

// Info returns metadata about this hook
func (h *MagicWordHook) Info() hooks.HookInfo {
	return hooks.HookInfo{
		Name:        "magic_word_hook",
		Version:     "1.0.0", 
		Description: "Injects magic words into conversation content for LLM behavior testing",
	}
}
```

### Magic Word Generation

```go
// generateMagicWord creates a new random magic word and stores it
func (h *MagicWordHook) generateMagicWord() string {
	if !h.enabled {
		return ""
	}
	
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(h.wordBank))))
	if err != nil {
		h.logger.Warn("Failed to generate random magic word", "error", err)
		return "FALLBACK"
	}
	
	word := h.wordBank[n.Int64()]
	h.currentMagicWord = word
	h.logger.Info("Generated new magic word", "word", word)
	return word
}

// createMagicMessage creates the magic word message to append
func (h *MagicWordHook) createMagicMessage() string {
	word := h.generateMagicWord()
	return fmt.Sprintf("\n\nHELLO THE MAGIC WORD IS %s", word)
}
```

### Content Mutation Implementation

```go
// MutateToolResult modifies tool results to include magic word
func (h *MagicWordHook) MutateToolResult(ctx context.Context, r *hooks.ToolResultMutationCtx) (hooks.TransformResult[*hooks.ToolResultMutationCtx], error) {
	if !h.enabled || r == nil {
		return hooks.TransformResult[*hooks.ToolResultMutationCtx]{}, nil
	}
	
	// Only inject into successful tool results 
	if r.IsError {
		return hooks.TransformResult[*hooks.ToolResultMutationCtx]{}, nil
	}
	
	// Skip certain tools that should not be mutated
	skipTools := []string{"edit_file", "create_file", "bash"} // Critical tools
	for _, skipTool := range skipTools {
		if r.ToolName == skipTool {
			h.logger.Debug("Skipping mutation for critical tool", "tool", r.ToolName)
			return hooks.TransformResult[*hooks.ToolResultMutationCtx]{}, nil
		}
	}
	
	magicMessage := h.createMagicMessage()
	mutatedContent := r.Content + magicMessage
	
	h.logger.Info("Mutated tool result", 
		"session_id", r.SessionID,
		"tool_name", r.ToolName,
		"tool_call_id", r.ToolCallID,
		"magic_word", h.currentMagicWord,
		"original_length", len(r.Content),
		"mutated_length", len(mutatedContent),
	)
	
	// Return new context with mutated content
	return hooks.TransformResult[*hooks.ToolResultMutationCtx]{
		Modified: &hooks.ToolResultMutationCtx{
			SessionID:  r.SessionID,
			AgentID:    r.AgentID,
			ToolName:   r.ToolName,
			ToolCallID: r.ToolCallID,
			Content:    mutatedContent,
			IsError:    r.IsError,
			Metadata:   r.Metadata,
		},
	}, nil
}
```

## Agent Integration Points

### Tool Result Mutation Integration

**Location**: `internal/llm/agent/agent.go` in `streamAndHandleEvents()` method

**Before** (line ~608):
```go
toolResults[i] = message.ToolResult{
	ToolCallID: toolCall.ID,
	Content:    toolResponse.Content,
	Metadata:   toolResponse.Metadata,
	IsError:    toolResponse.IsError,
}
```

**After**:
```go
// Apply tool result mutations via hooks
mutatedContent := toolResponse.Content
if a.hooks != nil {
	mutationCtx := &hooks.ToolResultMutationCtx{
		SessionID:  sessionID,
		AgentID:    a.agentCfg.ID,
		ToolName:   toolCall.Name,
		ToolCallID: toolCall.ID,
		Content:    toolResponse.Content,
		IsError:    toolResponse.IsError,
		Metadata:   toolResponse.Metadata,
	}
	
	result := a.hooks.EmitToolResultMutation(ctx, mutationCtx)
	if result != nil && result.Modified != nil {
		mutatedContent = result.Modified.Content
	}
}

toolResults[i] = message.ToolResult{
	ToolCallID: toolCall.ID,
	Content:    mutatedContent,
	Metadata:   toolResponse.Metadata,
	IsError:    toolResponse.IsError,
}
```

### LLM Response Mutation Integration

**Location**: `internal/llm/agent/agent.go` in `processEvent()` method

**Target**: `provider.EventContentDelta` case for streaming content

**Implementation**: Add mutation after content delta processing to modify final LLM response content before storage.

## Safety Mechanisms

### 1. Environment Gating
```go
enabled: os.Getenv("CRUSH_USE_MAGIC_WORD_HOOK") == "1"
```

### 2. Tool Filtering
```go
// Critical tools that should never be mutated
skipTools := []string{
	"edit_file",    // File modifications should be exact
	"create_file",  // File creation should be exact  
	"bash",         // Shell commands should be exact
	"grep",         // Search results should be exact
}
```

### 3. Error Handling
```go
// Skip mutation for error results
if r.IsError {
	return r
}

// Panic recovery in hook manager
defer func() {
	if r := recover(); r != nil {
		slog.Warn("Hook panic recovered during mutation", "panic", r)
		// Return original content on panic
	}
}()
```

### 4. Logging & Observability
```go
h.logger.Info("Mutated tool result", 
	"session_id", r.SessionID,
	"tool_name", r.ToolName, 
	"magic_word", h.currentMagicWord,
	"original_length", len(r.Content),
	"mutated_length", len(mutatedContent),
)
```

### 5. Content Size Limits
```go
// Prevent excessive content growth
const maxContentSize = 100_000 // 100KB limit

if len(mutatedContent) > maxContentSize {
	h.logger.Warn("Mutation would exceed size limit, skipping", 
		"original_size", len(r.Content),
		"limit", maxContentSize)
	return r
}
```

## Testing Strategy

### 1. Unit Tests
```go
// Test hook creation and configuration
func TestMagicWordHook_New(t *testing.T)

// Test magic word generation
func TestMagicWordHook_GenerateMagicWord(t *testing.T)

// Test tool result mutation
func TestMagicWordHook_MutateToolResult(t *testing.T)

// Test safety mechanisms (error handling, tool filtering)
func TestMagicWordHook_SafetyMechanisms(t *testing.T)
```

### 2. Integration Tests
```go
// Test hook integration with agent
func TestAgent_WithMagicWordHook(t *testing.T)

// Test hook manager mutation emission
func TestHookManager_ToolResultMutation(t *testing.T)
```

### 3. Manual Testing Protocol

1. **Setup**: `export CRUSH_USE_MAGIC_WORD_HOOK=1`
2. **Test Tool Call**: Execute `list_directory` or `read_file` 
3. **Verify Injection**: Check that tool result contains "HELLO THE MAGIC WORD IS XXX"
4. **Test LLM Response**: Verify LLM acknowledges/responds to magic word
5. **Test Conversation Flow**: Ensure conversation continues normally
6. **Test Safety**: Verify critical tools (edit_file, bash) are not mutated

### 4. LLM Behavior Validation

**Test Cases**:
- Does the LLM acknowledge the magic word?
- Does it ask about the magic word?
- Does it ignore the magic word appropriately?
- Does conversation quality degrade with injected content?

## Implementation Phases

### Phase 1: Core Infrastructure
- [ ] Add new hook interfaces to `internal/hooks/hooks.go`
- [ ] Extend hook manager with mutation emission methods
- [ ] Add new event context structs

### Phase 2: Magic Word Hook
- [ ] Implement `internal/hooks/builtin/magicword/magicword.go`
- [ ] Add magic word generation and injection logic
- [ ] Implement safety mechanisms

### Phase 3: Agent Integration  
- [ ] Modify `streamAndHandleEvents()` for tool result mutation
- [ ] Add hook emission calls at appropriate points
- [ ] Test integration with existing hook system

### Phase 4: Testing & Validation
- [ ] Write comprehensive unit tests
- [ ] Add integration tests with agent
- [ ] Manual testing with real conversations
- [ ] Document LLM behavior observations

### Phase 5: Registration & Configuration
- [ ] Add hook registration to `internal/app/app.go`
- [ ] Environment variable configuration
- [ ] Documentation and usage examples

## Future Extensions

This design provides foundation for:

1. **Response Mutation**: Modifying LLM responses before display
2. **Input Mutation**: Preprocessing user input before LLM
3. **Conversation Injection**: Adding context or instructions mid-conversation
4. **A/B Testing**: Different content mutations for experimentation
5. **Content Filtering**: Removing or replacing sensitive content
6. **Behavioral Testing**: Systematic testing of LLM responses to various stimuli

## Risk Assessment

**Low Risk**:
- Read-only observation hooks (current system)
- Magic word injection in non-critical tool results

**Medium Risk**:
- Tool result mutation for critical tools (file operations, shell commands)
- LLM response mutation affecting conversation quality

**High Risk**:
- User input mutation (could break user intent)
- Content mutation without proper safety guards
- Mutation of system-critical tool outputs

**Mitigation**: Start with low-risk tool result mutation, extensive testing, and conservative safety mechanisms.
