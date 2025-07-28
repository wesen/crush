# Streaming Events Research for Crush Coding Agent

## Executive Summary

This document provides an in-depth analysis of the streaming events system in the Crush coding agent, covering how LLM providers generate real-time events, how the agent processes and transforms them, and how the UI consumes them to provide live updates. The streaming system enables real-time user feedback for text generation, reasoning display, tool call execution, and error handling.

## Table of Contents

1. [Streaming Events Overview](#streaming-events-overview)
2. [Event Types and Data Structures](#event-types-and-data-structures)
3. [Provider-Level Event Generation](#provider-level-event-generation)
4. [Agent-Level Event Processing](#agent-level-event-processing)
5. [Message Accumulation and State Management](#message-accumulation-and-state-management)
6. [UI Event Consumption and Display](#ui-event-consumption-and-display)
7. [Pub/Sub Infrastructure](#pubsub-infrastructure)
8. [Tool Execution Integration](#tool-execution-integration)
9. [Error Handling and Recovery](#error-handling-and-recovery)
10. [Performance Considerations](#performance-considerations)

## Streaming Events Overview

The Crush agent implements a sophisticated streaming events system that enables real-time updates throughout the LLM conversation lifecycle. The system operates across several layers:

1. **Provider Layer**: LLM providers (Anthropic, OpenAI, Gemini, etc.) generate streaming events from their APIs
2. **Agent Layer**: The agent processes provider events and manages message state
3. **Message Layer**: Messages accumulate content incrementally and publish updates
4. **UI Layer**: The TUI consumes message updates and renders live changes

### Event Flow Architecture

```
LLM API Stream → Provider Events → Agent Processing → Message Updates → Pub/Sub → UI Updates
```

This architecture allows for:
- **Real-time text streaming**: Users see text appear character by character
- **Live reasoning display**: Anthropic's "thinking" process is shown in real-time
- **Progressive tool calls**: Tool call arguments build up incrementally
- **Immediate error feedback**: Errors are displayed as soon as they occur

## Event Types and Data Structures

### Core Event Types

The system defines 11 distinct event types in `internal/llm/provider/provider.go`:

```go
const (
    EventContentStart   EventType = "content_start"   // Text content begins
    EventToolUseStart   EventType = "tool_use_start"  // Tool call initiated  
    EventToolUseDelta   EventType = "tool_use_delta"  // Tool arguments accumulating
    EventToolUseStop    EventType = "tool_use_stop"   // Tool call complete
    EventContentDelta   EventType = "content_delta"   // Incremental text content
    EventThinkingDelta  EventType = "thinking_delta"  // Reasoning content (Anthropic)
    EventSignatureDelta EventType = "signature_delta" // Signature content (Anthropic)
    EventContentStop    EventType = "content_stop"    // Text content complete
    EventComplete       EventType = "complete"        // Full response complete
    EventError          EventType = "error"           // Error occurred
    EventWarning        EventType = "warning"         // Warning issued
)
```

### Event Data Structure

Each streaming event carries specific payload data:

```go
type ProviderEvent struct {
    Type EventType

    // Content fields
    Content   string              // Text content delta
    Thinking  string              // Reasoning content delta
    Signature string              // Signature content delta
    
    // Structured data
    Response  *ProviderResponse   // Final response (EventComplete)
    ToolCall  *message.ToolCall   // Tool call data
    Error     error              // Error information
}
```

### Provider Response Structure

The final response contains aggregated information:

```go
type ProviderResponse struct {
    Content      string                // Complete text content
    ToolCalls    []message.ToolCall    // All tool calls
    Usage        TokenUsage            // Token consumption
    FinishReason message.FinishReason  // Completion reason
}

type TokenUsage struct {
    InputTokens         int64  // Prompt tokens
    OutputTokens        int64  // Generated tokens  
    CacheCreationTokens int64  // Cache creation cost
    CacheReadTokens     int64  // Cache read savings
}
```

## Provider-Level Event Generation

Each LLM provider implements streaming differently, but all emit standardized `ProviderEvent` structures.

### Anthropic Provider Streaming

Anthropic's Claude models support advanced features like "thinking" (reasoning) and structured tool calls:

```go
// From internal/llm/provider/anthropic.go
case anthropic.ContentBlockDeltaEvent:
    if event.Delta.Type == "thinking_delta" && event.Delta.Thinking != "" {
        eventChan <- ProviderEvent{
            Type:     EventThinkingDelta,
            Thinking: event.Delta.Thinking,
        }
    } else if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
        eventChan <- ProviderEvent{
            Type:    EventContentDelta,
            Content: event.Delta.Text,
        }
    } else if event.Delta.Type == "input_json_delta" {
        if currentToolCallID != "" {
            eventChan <- ProviderEvent{
                Type: EventToolUseDelta,
                ToolCall: &message.ToolCall{
                    ID:       currentToolCallID,
                    Finished: false,
                    Input:    event.Delta.PartialJSON,
                },
            }
        }
    }
```

**Anthropic Event Sequence:**
1. `ContentBlockStartEvent` → `EventToolUseStart` or `EventContentStart`
2. `ContentBlockDeltaEvent` → `EventThinkingDelta`, `EventContentDelta`, or `EventToolUseDelta` 
3. `ContentBlockStopEvent` → `EventToolUseStop` or `EventContentStop`
4. `MessageCompleteEvent` → `EventComplete`

### OpenAI Provider Streaming

OpenAI uses a different streaming format focused on content and function calls:

```go
// From internal/llm/provider/openai.go
for _, choice := range chunk.Choices {
    if choice.Delta.Content != "" {
        eventChan <- ProviderEvent{
            Type:    EventContentDelta,
            Content: choice.Delta.Content,
        }
        currentContent += choice.Delta.Content
    } else if len(choice.Delta.ToolCalls) > 0 {
        toolCall := choice.Delta.ToolCalls[0]
        // Tool call detection and delta handling
        if currentToolCallID == "" {
            // New tool call started
            currentToolCallID = toolCall.ID
        } else {
            // Accumulate tool call arguments
            currentToolCall.Function.Arguments += toolCall.Function.Arguments
        }
    }
}
```

**OpenAI Event Sequence:**
1. Content deltas: `EventContentDelta` for each text chunk
2. Tool call deltas: `EventToolUseDelta` for function arguments
3. Completion: `EventComplete` with final response

### Gemini Provider Streaming

Gemini follows a similar pattern with its own API structure but maps to the same event types.

## Agent-Level Event Processing

The agent (`internal/llm/agent/agent.go`) acts as the central orchestrator, processing provider events and updating message state.

### Main Processing Loop

```go
// From internal/llm/agent/agent.go:streamAndHandleEvents()
eventChan := a.provider.StreamResponse(ctx, msgHistory, slices.Collect(a.tools.Seq()))

// Process each event in the stream
for event := range eventChan {
    if processErr := a.processEvent(ctx, sessionID, &assistantMsg, event); processErr != nil {
        // Handle errors and cancellation
        if errors.Is(processErr, context.Canceled) {
            a.finishMessage(context.Background(), &assistantMsg, message.FinishReasonCanceled, "Request cancelled", "")
        } else {
            a.finishMessage(ctx, &assistantMsg, message.FinishReasonError, "API Error", processErr.Error())
        }
        return assistantMsg, nil, processErr
    }
}
```

### Event Processing by Type

Each event type has specific handling logic:

```go
func (a *agent) processEvent(ctx context.Context, sessionID string, assistantMsg *message.Message, event provider.ProviderEvent) error {
    switch event.Type {
    case provider.EventThinkingDelta:
        assistantMsg.AppendReasoningContent(event.Thinking)
        return a.messages.Update(ctx, *assistantMsg)
        
    case provider.EventSignatureDelta:
        assistantMsg.AppendReasoningSignature(event.Signature)
        return a.messages.Update(ctx, *assistantMsg)
        
    case provider.EventContentDelta:
        assistantMsg.FinishThinking()  // Stop thinking animation
        assistantMsg.AppendContent(event.Content)
        return a.messages.Update(ctx, *assistantMsg)
        
    case provider.EventToolUseStart:
        assistantMsg.FinishThinking()
        slog.Info("Tool call started", "toolCall", event.ToolCall)
        assistantMsg.AddToolCall(*event.ToolCall)
        return a.messages.Update(ctx, *assistantMsg)
        
    case provider.EventToolUseDelta:
        assistantMsg.AppendToolCallInput(event.ToolCall.ID, event.ToolCall.Input)
        return a.messages.Update(ctx, *assistantMsg)
        
    case provider.EventToolUseStop:
        slog.Info("Finished tool call", "toolCall", event.ToolCall)
        assistantMsg.FinishToolCall(event.ToolCall.ID)
        return a.messages.Update(ctx, *assistantMsg)
        
    case provider.EventComplete:
        assistantMsg.FinishThinking()
        assistantMsg.SetToolCalls(event.Response.ToolCalls)
        assistantMsg.AddFinish(event.Response.FinishReason, "", "")
        if err := a.messages.Update(ctx, *assistantMsg); err != nil {
            return fmt.Errorf("failed to update message: %w", err)
        }
        return a.TrackUsage(ctx, sessionID, a.Model(), event.Response.Usage)
        
    case provider.EventError:
        return event.Error
    }
    return nil
}
```

### State Transitions

The agent manages several important state transitions:

1. **Thinking → Content**: When text starts, thinking animations stop
2. **Content Accumulation**: Text deltas are appended incrementally  
3. **Tool Call Lifecycle**: Tool calls progress from start → delta → stop
4. **Completion Handling**: Final state updates and usage tracking

## Message Accumulation and State Management

Messages maintain state and accumulate content incrementally as streaming events arrive.

### Message Content Structure

Messages contain various content types that can be updated incrementally:

```go
// From internal/message/content.go
type ReasoningContent struct {
    Thinking   string `json:"thinking"`       // Anthropic reasoning
    Signature  string `json:"signature"`      // Function signatures
    StartedAt  int64  `json:"started_at,omitempty"`
    FinishedAt int64  `json:"finished_at,omitempty"`
}

type TextContent struct {
    Text string `json:"text"`               // Main response text
}

type ToolCall struct {
    ID       string `json:"id"`             // Unique identifier
    Name     string `json:"name"`           // Tool name
    Input    string `json:"input"`          // JSON arguments
    Type     string `json:"type"`           // Call type
    Finished bool   `json:"finished"`       // Completion status
}
```

### Message Update Methods

Messages provide methods for incremental updates:

```go
// Content accumulation methods
assistantMsg.AppendContent(event.Content)                    // Add text delta
assistantMsg.AppendReasoningContent(event.Thinking)          // Add thinking delta
assistantMsg.AppendReasoningSignature(event.Signature)       // Add signature delta

// Tool call management
assistantMsg.AddToolCall(*event.ToolCall)                   // Start new tool call
assistantMsg.AppendToolCallInput(event.ToolCall.ID, input)  // Add to arguments
assistantMsg.FinishToolCall(event.ToolCall.ID)             // Mark complete

// State management
assistantMsg.FinishThinking()                               // Stop thinking animation
assistantMsg.AddFinish(reason, "", "")                     // Set finish reason
```

### Database Persistence

Each message update triggers a database write to persist the current state:

```go
// From internal/llm/agent/agent.go
case provider.EventContentDelta:
    assistantMsg.AppendContent(event.Content)
    return a.messages.Update(ctx, *assistantMsg)  // Database update
```

This ensures that partial progress is preserved even if the system crashes or is interrupted.

## UI Event Consumption and Display

The TUI consumes message updates through the pub/sub system and renders them in real-time.

### Message Event Handling

The chat component (`internal/tui/components/chat/chat.go`) handles different types of message events:

```go
func (m *messageListCmp) handleMessageEvent(event pubsub.Event[message.Message]) tea.Cmd {
    switch event.Type {
    case pubsub.CreatedEvent:
        if event.Payload.SessionID != m.session.ID {
            return m.handleChildSession(event)
        }
        if m.messageExists(event.Payload.ID) {
            return nil
        }
        return m.handleNewMessage(event.Payload)
        
    case pubsub.UpdatedEvent:
        if event.Payload.SessionID != m.session.ID {
            return m.handleChildSession(event)
        }
        return m.handleUpdateAssistantMessage(event.Payload)
    }
    return nil
}
```

### Live Content Rendering

The message component renders different content types with appropriate styling and animations:

```go
// From internal/tui/components/chat/messages/messages.go
func (m *messageCmp) renderAssistantMessage() string {
    content := m.message.Content().String()
    thinking := m.message.IsThinking()
    finished := m.message.IsFinished()
    
    if thinking || m.message.ReasoningContent().Thinking != "" {
        m.anim.SetLabel("Thinking")
        thinkingContent = m.renderThinkingContent()
    }
    
    // Render thinking content with animation
    if thinkingContent != "" {
        parts = append(parts, thinkingContent)
    }
    
    // Render main text content
    if content != "" {
        if thinkingContent != "" {
            parts = append(parts, "")  // Spacing
        }
        parts = append(parts, m.toMarkdown(content))
    }
    
    return m.style().Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}
```

### Thinking Content Display

Anthropic's reasoning content gets special treatment with a scrollable viewport:

```go
func (m *messageCmp) renderThinkingContent() string {
    reasoningContent := m.message.ReasoningContent()
    if reasoningContent.Thinking == "" {
        return ""
    }
    
    // Format thinking content with special styling
    lines := strings.Split(reasoningContent.Thinking, "\n")
    lineStyle := t.S().Subtle.Background(t.BgBaseLighter)
    
    // Use viewport for scrollable thinking content
    height := util.Clamp(lipgloss.Height(fullContent), 1, 10)
    m.thinkingViewport.SetHeight(height)
    m.thinkingViewport.SetContent(fullContent)
    m.thinkingViewport.GotoBottom()  // Auto-scroll to latest
    
    // Show duration when thinking is complete
    if reasoningContent.FinishedAt > 0 {
        duration := m.message.ThinkingDuration()
        opts := core.StatusOpts{
            Title:       "Thought for",
            Description: duration.String(),
            NoIcon:      true,
        }
        return core.Status(opts, m.textWidth()-1)
    }
    
    return lineStyle.Width(m.textWidth()).Padding(0, 1).Render(m.thinkingViewport.View())
}
```

### Tool Call Display

Tool calls are rendered as separate components with progressive argument display:

```go
func (m *messageListCmp) updateOrAddToolCall(msg message.Message, tc message.ToolCall, existingToolCalls map[int]messages.ToolCallCmp) tea.Cmd {
    // Try to find existing tool call
    for index, existingTC := range existingToolCalls {
        if tc.ID == existingTC.GetToolCall().ID {
            existingTC.SetToolCall(tc)  // Update arguments
            if msg.FinishPart() != nil && msg.FinishPart().Reason == message.FinishReasonCanceled {
                existingTC.SetCancelled()
            }
            m.listCmp.UpdateItem(index, existingTC)
            return nil
        }
    }
    
    // Add new tool call if not found
    return m.listCmp.AppendItem(messages.NewToolCallCmp(msg.ID, tc))
}
```

### Spin Animation Management

The UI shows loading animations for active operations:

```go
func (m *messageCmp) shouldSpin() bool {
    if m.message.Role != message.Assistant {
        return false
    }
    
    if m.message.IsFinished() {
        return false
    }
    
    if m.message.Content().Text != "" {
        return false
    }
    
    if len(m.message.ToolCalls()) > 0 {
        return false
    }
    
    return true  // Show spinner for empty assistant messages
}
```

## Pub/Sub Infrastructure

The system uses a generic pub/sub broker to distribute message updates across the application.

### Broker Architecture

```go
// From internal/pubsub/broker.go
type Broker[T any] struct {
    subs      map[chan Event[T]]struct{}  // Active subscriptions
    mu        sync.RWMutex                // Thread safety
    done      chan struct{}               // Shutdown signal
    subCount  int                         // Subscriber count
    maxEvents int                         // Buffer limits
}
```

### Event Distribution

The broker efficiently distributes events to all subscribers:

```go
func (b *Broker[T]) Publish(t EventType, payload T) {
    b.mu.RLock()
    subscribers := make([]chan Event[T], 0, len(b.subs))
    for sub := range b.subs {
        subscribers = append(subscribers, sub)
    }
    b.mu.RUnlock()
    
    event := Event[T]{Type: t, Payload: payload}
    
    for _, sub := range subscribers {
        select {
        case sub <- event:
        default:
            // Channel is full, subscriber is slow - skip this event
            // This prevents blocking the publisher
        }
    }
}
```

### Message Service Integration

The message service publishes updates after each database write:

```go
// From internal/message/message.go
func (s *service) Update(ctx context.Context, message Message) error {
    // Database update logic...
    
    s.Publish(pubsub.UpdatedEvent, message)  // Notify subscribers
    return nil
}
```

## Tool Execution Integration

While tool execution itself doesn't generate streaming events, the results are integrated into the streaming flow.

### Tool Execution Flow

1. **Tool Call Detection**: LLM generates tool call through streaming events
2. **Argument Accumulation**: Tool arguments build up via `EventToolUseDelta`
3. **Execution Trigger**: When tool call is complete, agent executes it
4. **Result Integration**: Tool results are added to message and published

### Tool Result Processing

```go
// From internal/llm/agent/agent.go:streamAndHandleEvents()
for i, toolCall := range toolCalls {
    tool, exists := a.tools.Get(toolCall.Name)
    if !exists {
        // Handle unknown tool
        continue
    }
    
    // Execute tool (blocking operation)
    toolResponse, toolErr := tool.Run(ctx, tools.ToolCall{
        ID:    toolCall.ID,
        Name:  toolCall.Name,
        Input: toolCall.Input,
    })
    
    // Create tool result message
    toolMessage, err := a.messages.Create(ctx, sessionID, message.CreateMessageParams{
        Role: message.Tool,
        Parts: []message.ContentPart{
            message.ToolResult{
                ToolCallID: toolCall.ID,
                Name:       toolCall.Name,
                Content:    toolResponse.Content,
                IsError:    toolResponse.IsError,
            },
        },
    })
    
    // This triggers pub/sub events for UI updates
}
```

### Tool Result Display

Tool results appear as separate message components in the UI, with appropriate formatting for different tool types (bash output, file contents, etc.).

## Error Handling and Recovery

The streaming system includes comprehensive error handling at multiple levels.

### Provider-Level Errors

Providers emit `EventError` events for API failures:

```go
case provider.EventError:
    return event.Error  // Propagate to agent
```

### Agent-Level Error Handling

The agent processes errors and updates message state:

```go
if processErr := a.processEvent(ctx, sessionID, &assistantMsg, event); processErr != nil {
    if errors.Is(processErr, context.Canceled) {
        a.finishMessage(context.Background(), &assistantMsg, message.FinishReasonCanceled, "Request cancelled", "")
    } else {
        a.finishMessage(ctx, &assistantMsg, message.FinishReasonError, "API Error", processErr.Error())
    }
    return assistantMsg, nil, processErr
}
```

### Cancellation Support

The system supports graceful cancellation at any point:

```go
// Context cancellation check in event processing
select {
case <-ctx.Done():
    return ctx.Err()
default:
    // Continue processing
}
```

### UI Error Display

Errors are displayed with special formatting in the UI:

```go
func (m *messageCmp) renderAssistantMessage() string {
    if finished && content == "" && finishedData.Reason == message.FinishReasonError {
        errTag := t.S().Base.Padding(0, 1).Background(t.Red).Foreground(t.White).Render("ERROR")
        truncated := ansi.Truncate(finishedData.Message, m.textWidth()-2-lipgloss.Width(errTag), "...")
        title := fmt.Sprintf("%s %s", errTag, t.S().Base.Foreground(t.FgHalfMuted).Render(truncated))
        details := t.S().Base.Foreground(t.FgSubtle).Width(m.textWidth() - 2).Render(finishedData.Details)
        return fmt.Sprintf("%s\n\n%s", title, details)
    }
}
```

## Performance Considerations

The streaming system is designed for efficiency and responsiveness.

### Event Buffering

- **Provider Level**: Events are generated as fast as the API provides them
- **Pub/Sub Level**: 64-event buffer prevents blocking publishers
- **UI Level**: Bubble Tea's command system batches updates

### Database Performance

Each streaming event triggers a database update, but this is optimized:

- **SQLite Performance**: Fast local database with appropriate indexes
- **Incremental Updates**: Only changed fields are written
- **Transaction Efficiency**: Updates are quick and don't block streaming

### Memory Management

- **Event Cleanup**: Completed events are not retained in memory
- **Message State**: Only current message state is kept
- **Tool Results**: Large tool outputs may be truncated for display

### UI Rendering Optimization

- **Differential Updates**: Only changed components re-render
- **Viewport Scrolling**: Large content uses scrollable viewports
- **Animation Efficiency**: Thinking animations are lightweight

## Conclusion

The Crush agent's streaming events system provides a sophisticated foundation for real-time user interaction. Key strengths include:

1. **Multi-Provider Support**: Unified event model across different LLM APIs
2. **Real-Time Feedback**: Immediate display of text, thinking, and tool calls
3. **Robust Error Handling**: Graceful degradation and cancellation support
4. **Efficient Architecture**: Optimized for performance and responsiveness
5. **Extensible Design**: Easy to add new event types and providers

The system enables users to see:
- **Live text generation** as the LLM writes responses
- **Reasoning processes** from advanced models like Claude
- **Tool call formation** as arguments are built incrementally  
- **Tool execution results** as they complete
- **Error states** with clear visual feedback

This streaming architecture is crucial for the interactive coding experience that Crush provides, allowing users to understand what the agent is doing at each step and providing immediate feedback throughout the conversation lifecycle. 