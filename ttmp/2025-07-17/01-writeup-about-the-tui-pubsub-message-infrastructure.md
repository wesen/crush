# TUI PubSub Message Infrastructure: Technical Deep Dive

## Executive Summary

The Crush codebase implements a sophisticated, type-safe publish-subscribe architecture that serves as the backbone for real-time TUI updates. Built on Go generics, the system provides seamless state synchronization between backend services and frontend UI components without direct coupling.

## Architecture Overview

### Core Design Philosophy

The pubsub system follows a **domain-driven design** where each service domain (messages, sessions, files, permissions) maintains its own event stream while the TUI layer acts as a coordinated subscriber. This creates a **clean separation of concerns** between data mutation and UI updates.

### System Components Map

```
┌─────────────────────────────────────────────────────────────┐
│                        Application Layer                    │
├─────────────────────────────────────────────────────────────┤
│  Chat Page    │  Header    │  Sidebar    │  File Picker   │
│  (Messages)   │  (Sessions)│  (Sessions) │  (Files)      │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      Event Router Layer                     │
│                    (App → Component Routing)              │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      Service Layer                        │
│  Messages  │  Sessions  │  Files  │  Permissions      │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      PubSub Core                          │
│                 Generic Broker[T] Implementation            │
└─────────────────────────────────────────────────────────────┘
```

## Core PubSub Implementation

### Generic Broker Design

The `Broker[T]` struct in `/internal/pubsub/broker.go` represents a masterclass in Go generic programming:

```go
type Broker[T any] struct {
    mu          sync.RWMutex
    subscribers map[chan Event[T]]struct{}
    bufferSize  int
}

func (b *Broker[T]) Subscribe(ctx context.Context) <-chan Event[T] {
    ch := make(chan Event[T], b.bufferSize)
    b.mu.Lock()
    b.subscribers[ch] = struct{}{}
    b.mu.Unlock()
    
    go func() {
        <-ctx.Done()
        b.removeSubscriber(ch)
    }()
    
    return ch
}
```

**Key Features:**
- **Type safety** through Go generics
- **Memory efficiency** with configurable buffer sizes
- **Automatic cleanup** via context cancellation
- **Thread-safe** concurrent access patterns

### Event Type System

The system uses a **semantic event model** rather than simple notifications:

```go
type EventType string

const (
    CreatedEvent EventType = "created"
    UpdatedEvent EventType = "updated"
    DeletedEvent EventType = "deleted"
)

type Event[T any] struct {
    Type    EventType
    Payload T
}
```

This approach enables **fine-grained updates** - components can respond differently to creation vs. updates vs. deletions of the same entity type.

## Service Integration Patterns

### Publisher Interface

Each service implements a consistent publisher pattern:

```go
type Publisher[T any] interface {
    Subscribe(ctx context.Context) <-chan pubsub.Event[T]
    Publish(eventType pubsub.EventType, payload T)
}
```

**Service Implementations:**
- **Messages**: Real-time chat message updates
- **Sessions**: Session lifecycle events
- **Files**: File operation notifications
- **Permissions**: Permission request handling
- **Agents**: Agent lifecycle management

### Message Flow Example

Consider a typical message creation flow:

1. **User Input**: User types message in chat
2. **Service Layer**: Message service creates new message
3. **Event Publication**: Message service publishes `CreatedEvent`
4. **Global Routing**: App layer receives event and routes to components
5. **UI Update**: Message list component adds new message
6. **State Sync**: All subscribers update their state accordingly

```go
// Service layer
func (s *MessageService) CreateMessage(msg message.Message) error {
    // ... persist message ...
    s.Publish(pubsub.CreatedEvent, msg)
    return nil
}

// TUI layer
func (m *messageListCmp) handleMessageEvent(event pubsub.Event[message.Message]) tea.Cmd {
    switch event.Type {
    case pubsub.CreatedEvent:
        return m.handleNewMessage(event.Payload)
    case pubsub.UpdatedEvent:
        return m.handleUpdateAssistantMessage(event.Payload)
    }
    return nil
}
```

## TUI Integration Architecture

### Event Bridge Pattern

The **App layer** implements a sophisticated event bridge that translates between service events and TUI messages:

```go
func (a *App) setupSubscribers(ctx context.Context) tea.Cmd {
    var cmds []tea.Cmd
    
    // Generic subscriber setup
    cmds = append(cmds, setupSubscriber(ctx, &a.wg, "messages", 
        a.Messages.Subscribe, a.pubSubCh))
    cmds = append(cmds, setupSubscriber(ctx, &a.wg, "sessions", 
        a.Sessions.Subscribe, a.pubSubCh))
    cmds = append(cmds, setupSubscriber(ctx, &a.wg, "files", 
        a.Files.Subscribe, a.pubSubCh))
    
    return tea.Batch(cmds...)
}
```

### Component-Level Subscription

Components subscribe to relevant event streams through a **hierarchical routing system**:

```go
// Global level - App subscribes to all services
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch event := msg.(type) {
    case pubsub.Event[message.Message]:
        return a.handleMessageEvent(event)
    case pubsub.Event[session.Session]:
        return a.handleSessionEvent(event)
    }
}

// Page level - Routes to specific components
func (p *ChatPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case pubsub.Event[message.Message]:
        u, cmd := p.chat.Update(msg)  // Route to message list
        p.chat = u.(chat.MessageListCmp)
        
    case pubsub.Event[session.Session]:
        u, cmd := p.header.Update(msg)  // Route to header
        u, cmd = p.sidebar.Update(msg)  // Route to sidebar
    }
}
```

## Advanced Event Processing

### Nested Tool Call Handling

The system handles complex nested operations like tool calls through **hierarchical message routing**:

```go
// Parent session receives tool call event
if event.Payload.Type == "tool_call" {
    // Create child session for tool execution
    childSession := session.NewChildSession(event.Payload)
    
    // Route tool results back to parent
    return m.handleToolResult(childSession)
}

// Tool results propagate back to parent message
func (m *messageListCmp) handleToolResult(childSession *session.Session) tea.Cmd {
    // Update parent message with tool results
    parentMessage := m.findMessageByID(childSession.ParentID)
    parentMessage.Content += childSession.Results
    return m.updateMessage(parentMessage)
}
```

### State Synchronization Patterns

**Real-time synchronization** happens through several mechanisms:

1. **Immediate Updates**: New messages appear instantly across all components
2. **Selective Re-rendering**: Only affected components update their display
3. **State Consistency**: All views maintain consistent state across the application
4. **Conflict Resolution**: Last-writer-wins for concurrent updates

## Performance Characteristics

### Memory Management

- **Buffer sizing**: Configurable buffer sizes prevent memory exhaustion
- **Subscriber cleanup**: Automatic cleanup prevents memory leaks
- **Selective fan-out**: Only active subscribers receive events

### Scaling Properties

- **Subscriber counting**: `GetSubscriberCount()` enables monitoring
- **Graceful degradation**: Slow consumers are skipped without blocking
- **Circuit breaker**: Automatic cleanup on context cancellation

**Performance Metrics:**
- **Buffer size**: Default 64 items per subscriber
- **Timeout**: 2-second timeout for slow consumers
- **Concurrency**: Thread-safe with minimal lock contention

### Bottleneck Mitigation

The system implements several strategies to handle high-volume scenarios:

1. **Buffered channels** prevent publisher blocking
2. **Selective delivery** skips inactive subscribers
3. **Timeout protection** prevents indefinite blocking
4. **Graceful shutdown** ensures proper cleanup

## Error Handling Patterns

### Resilient Event Processing

```go
// Non-blocking publish implementation
func (b *Broker[T]) Publish(eventType EventType, payload T) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    
    event := Event[T]{Type: eventType, Payload: payload}
    for ch := range b.subscribers {
        select {
        case ch <- event:
            // Delivered successfully
        default:
            // Channel full or consumer slow - skip to prevent blocking
            log.Debug("Skipping slow consumer")
        }
    }
}
```

### Consumer Timeout Handling

```go
func setupSubscriber[T any](ctx context.Context, 
    subscriber func(context.Context) <-chan pubsub.Event[T], 
    outputCh chan<- tea.Msg) tea.Cmd {
    return func() tea.Msg {
        events := subscriber(ctx)
        
        select {
        case event := <-events:
            return event
        case <-time.After(2 * time.Second):
            return nil // Timeout protection
        }
    }
}
```

## Testing and Monitoring

### Observable Events

The system provides several observability hooks:

- **Subscriber count**: Monitor active subscribers per service
- **Event frequency**: Track event rates by type and service
- **Delivery latency**: Measure end-to-end event delivery time
- **Error rates**: Monitor skipped consumers and delivery failures

### Testing Strategies

**Integration testing** patterns:

```go
func TestMessageFlow(t *testing.T) {
    // Setup test broker
    broker := pubsub.NewBroker[message.Message]()
    
    // Subscribe to events
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    events := broker.Subscribe(ctx)
    
    // Publish test event
    testMsg := message.NewMessage("Hello, World!")
    broker.Publish(pubsub.CreatedEvent, testMsg)
    
    // Verify delivery
    select {
    case event := <-events:
        assert.Equal(t, pubsub.CreatedEvent, event.Type)
        assert.Equal(t, testMsg.ID, event.Payload.ID)
    case <-time.After(1 * time.Second):
        t.Fatal("Event not delivered")
    }
}
```

## Conclusion

The pubsub architecture in Crush represents a sophisticated approach to real-time TUI development. By leveraging Go's generic programming capabilities, the system achieves **type-safe**, **performant**, and **scalable** event handling that enables seamless state synchronization between backend services and frontend UI components.

The design successfully balances **complexity management** with **performance requirements**, providing a robust foundation for real-time collaborative editing and asynchronous processing patterns. The generic broker implementation serves as an excellent example of how modern Go patterns can create elegant, reusable abstractions for complex system architectures.