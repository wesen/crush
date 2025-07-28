# LLM Response Parsing and Tool Execution: A Complete Guide

## Overview

The Crush TUI implements a sophisticated system for parsing LLM responses, detecting tool calls, executing them, and displaying structured results. This guide covers the complete flow from raw LLM output to user-friendly visualizations.

## Response Parsing Architecture

### Provider-Specific Parsers

The system supports multiple LLM providers with provider-specific response formats:

```go
type ResponseParser interface {
    ParseResponse(rawResponse string) (*ParsedResponse, error)
    ParseStreamingResponse(chunk string) (*StreamingChunk, error)
    ExtractToolCalls(content string) ([]ToolCall, error)
}
```

#### OpenAI Response Format

```go
type OpenAIResponse struct {
    Choices []struct {
        Message struct {
            Content string        `json:"content"`
            ToolCalls []ToolCall `json:"tool_calls"`
        } `json:"message"`
    } `json:"choices"`
}

func (p *OpenAIParser) ParseResponse(raw string) (*ParsedResponse, error) {
    var response OpenAIResponse
    if err := json.Unmarshal([]byte(raw), &response); err != nil {
        return nil, fmt.Errorf("invalid OpenAI response: %w", err)
    }
    
    return &ParsedResponse{
        Content:   response.Choices[0].Message.Content,
        ToolCalls: p.extractToolCalls(response.Choices[0].Message.ToolCalls),
    }, nil
}
```

#### Anthropic Response Format

```go
type AnthropicResponse struct {
    Content []struct {
        Type string `json:"type"`
        Text string `json:"text,omitempty"`
        ToolUse *struct {
            ID   string                 `json:"id"`
            Name string                 `json:"name"`
            Args map[string]interface{} `json:"input"`
        } `json:"tool_use,omitempty"`
    } `json:"content"`
}
```

#### Streaming Response Parsing

For real-time streaming responses:

```go
type StreamingParser struct {
    buffer    strings.Builder
    toolCalls []ToolCall
    state     ParseState
}

type ParseState int

const (
    StateContent ParseState = iota
    StateToolCall
    StateToolArguments
)

func (s *StreamingParser) ParseChunk(chunk string) (*StreamingChunk, error) {
    s.buffer.WriteString(chunk)
    
    // Detect tool call patterns in streaming content
    if s.detectToolCallStart(chunk) {
        s.state = StateToolCall
    }
    
    return &StreamingChunk{
        Content:   s.extractContent(),
        ToolCalls: s.extractToolCalls(),
        IsComplete: s.isComplete(),
    }, nil
}
```

## Tool Call Detection

### Pattern Matching

The system uses sophisticated pattern matching to detect tool calls:

```go
type ToolCallDetector struct {
    patterns []*ToolPattern
}

type ToolPattern struct {
    Name        string
    Regex       *regexp.Regexp
    StartMarker string
    EndMarker   string
}

var defaultPatterns = []*ToolPattern{
    {
        Name: "openai_function",
        Regex: regexp.MustCompile(`"function_call":\s*{\s*"name":\s*"([^"]+)"`),
        StartMarker: "```json",
        EndMarker: "```",
    },
    {
        Name: "anthropic_tool",
        Regex: regexp.MustCompile(`<tool_use>(.*?)</tool_use>`),
        StartMarker: "<tool_use>",
        EndMarker: "</tool_use>",
    },
}
```

### JSON Schema Validation

Tool calls are validated against JSON schemas:

```go
type ToolSchema struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]interface{}  `json:"parameters"`
    Required    []string               `json:"required"`
}

func (v *ToolValidator) ValidateCall(call ToolCall, schema ToolSchema) error {
    // Validate required parameters
    for _, param := range schema.Required {
        if _, exists := call.Arguments[param]; !exists {
            return fmt.Errorf("missing required parameter: %s", param)
        }
    }
    
    // Validate parameter types
    return v.validateTypes(call.Arguments, schema.Parameters)
}
```

## Tool Call Execution Flow

### Execution Pipeline

The complete tool execution pipeline:

```go
type ToolExecutor struct {
    registry *ToolRegistry
    executor *Executor
}

func (e *ToolExecutor) ExecuteToolCall(call ToolCall) (*ToolResult, error) {
    // 1. Resolve tool
    tool, err := e.registry.Get(call.Name)
    if err != nil {
        return nil, fmt.Errorf("tool not found: %s", call.Name)
    }
    
    // 2. Validate arguments
    if err := tool.Validate(call.Arguments); err != nil {
        return nil, fmt.Errorf("invalid arguments: %w", err)
    }
    
    // 3. Execute with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    result, err := tool.Execute(ctx, call.Arguments)
    if err != nil {
        return nil, fmt.Errorf("tool execution failed: %w", err)
    }
    
    // 4. Format result
    return e.formatResult(result), nil
}
```

### Tool Registry

```go
type ToolRegistry struct {
    tools map[string]Tool
}

func (r *ToolRegistry) Register(tool Tool) {
    r.tools[tool.Name()] = tool
}

func (r *ToolRegistry) Get(name string) (Tool, error) {
    tool, exists := r.tools[name]
    if !exists {
        return nil, fmt.Errorf("tool %s not found", name)
    }
    return tool, nil
}
```

### Built-in Tools

```go
// Available tools
var builtinTools = []Tool{
    &FileTool{
        Name: "create_file",
        Description: "Create a new file",
        Parameters: map[string]interface{}{
            "path": "string",
            "content": "string",
        },
    },
    
    &ExecuteTool{
        Name: "execute_command",
        Description: "Execute a shell command",
        Parameters: map[string]interface{}{
            "command": "string",
            "working_dir": "string",
        },
    },
    
    &WebTool{
        Name: "fetch_url",
        Description: "Fetch content from a URL",
        Parameters: map[string]interface{}{
            "url": "string",
            "selector": "string",
        },
    },
}
```

## Structured Data Display

### JSON Visualization

```go
type JSONRenderer struct {
    theme Theme
}

func (r *JSONRenderer) RenderJSON(data interface{}) string {
    var sb strings.Builder
    
    switch v := data.(type) {
    case map[string]interface{}:
        r.renderObject(v, 0, &sb)
    case []interface{}:
        r.renderArray(v, 0, &sb)
    default:
        sb.WriteString(r.formatValue(v))
    }
    
    return sb.String()
}

func (r *JSONRenderer) renderObject(obj map[string]interface{}, indent int, sb *strings.Builder) {
    sb.WriteString("{\n")
    
    keys := make([]string, 0, len(obj))
    for k := range obj {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    
    for i, key := range keys {
        r.writeIndent(indent+1, sb)
        sb.WriteString(r.formatKey(key))
        sb.WriteString(": ")
        r.renderValue(obj[key], indent+1, sb)
        
        if i < len(keys)-1 {
            sb.WriteString(",")
        }
        sb.WriteString("\n")
    }
    
    r.writeIndent(indent, sb)
    sb.WriteString("}")
}
```

### Table Display

```go
type TableRenderer struct {
    theme Theme
}

func (r *TableRenderer) RenderTable(data [][]string, headers []string) string {
    var sb strings.Builder
    
    // Calculate column widths
    widths := r.calculateWidths(data, headers)
    
    // Render header
    r.renderRow(headers, widths, true, &sb)
    
    // Render separator
    r.renderSeparator(widths, &sb)
    
    // Render rows
    for _, row := range data {
        r.renderRow(row, widths, false, &sb)
    }
    
    return sb.String()
}

func (r *TableRenderer) renderRow(row []string, widths []int, isHeader bool, sb *strings.Builder) {
    sb.WriteString("| ")
    for i, cell := range row {
        if isHeader {
            sb.WriteString(r.theme.Header(cell))
        } else {
            sb.WriteString(r.formatCell(cell, widths[i]))
        }
        sb.WriteString(" | ")
    }
    sb.WriteString("\n")
}
```

### Code Block Rendering

```go
type CodeRenderer struct {
    highlighter *Highlighter
}

func (r *CodeRenderer) RenderCode(code string, language string) string {
    switch language {
    case "json":
        return r.renderJSON(code)
    case "python", "py":
        return r.renderPython(code)
    case "bash", "shell":
        return r.renderBash(code)
    default:
        return r.renderPlain(code)
    }
}

func (r *CodeRenderer) renderJSON(jsonStr string) string {
    var parsed interface{}
    if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
        return r.renderPlain(jsonStr)
    }
    
    pretty, _ := json.MarshalIndent(parsed, "", "  ")
    return r.highlighter.Highlight(string(pretty), "json")
}
```

## Streaming Tool Calls

### Real-time Tool Execution

```go
type StreamingToolExecutor struct {
    registry *ToolRegistry
    progress chan ToolProgress
}

type ToolProgress struct {
    ToolName string
    Status   string // "starting", "executing", "completed", "error"
    Message  string
    Result   interface{}
}

func (e *StreamingToolExecutor) ExecuteStreaming(call ToolCall) (<-chan ToolProgress, error) {
    progress := make(chan ToolProgress, 10)
    
    go func() {
        defer close(progress)
        
        progress <- ToolProgress{
            ToolName: call.Name,
            Status:   "starting",
            Message:  fmt.Sprintf("Starting %s...", call.Name),
        }
        
        // Execute tool
        result, err := e.executeTool(call)
        if err != nil {
            progress <- ToolProgress{
                ToolName: call.Name,
                Status:   "error",
                Message:  err.Error(),
            }
            return
        }
        
        progress <- ToolProgress{
            ToolName: call.Name,
            Status:   "completed",
            Result:   result,
        }
    }()
    
    return progress, nil
}
```

### Partial Tool Call Handling

```go
type PartialToolCall struct {
    Name       string
    Arguments  map[string]interface{}
    Confidence float64
    IsComplete bool
}

func (h *PartialHandler) HandlePartialCall(content string) (*PartialToolCall, error) {
    // Detect incomplete tool calls in streaming content
    if !strings.Contains(content, "}") {
        // Incomplete JSON
        return h.handleIncompleteJSON(content)
    }
    
    // Validate partial arguments
    args, err := h.parsePartialArguments(content)
    if err != nil {
        return nil, err
    }
    
    return &PartialToolCall{
        Name:      h.extractToolName(content),
        Arguments: args,
        IsComplete: h.isComplete(content),
    }, nil
}
```

## Error Handling

### Malformed Tool Calls

```go
type ErrorHandler struct {
    fallback FallbackStrategy
}

type FallbackStrategy int

const (
    FallbackSkip FallbackStrategy = iota
    FallbackPrompt
    FallbackManual
)

func (h *ErrorHandler) HandleMalformed(call ToolCall, err error) (*ErrorResult, error) {
    switch h.fallback {
    case FallbackSkip:
        return &ErrorResult{
            Type:    "skipped",
            Message: fmt.Sprintf("Skipping malformed tool call: %v", err),
        }, nil
        
    case FallbackPrompt:
        return &ErrorResult{
            Type:    "prompt",
            Message: fmt.Sprintf("Please fix the tool call: %v", err),
        }, nil
        
    case FallbackManual:
        return &ErrorResult{
            Type:    "manual",
            Message: "Manual intervention required",
        }, nil
    }
    
    return nil, fmt.Errorf("unhandled error: %w", err)
}
```

### Validation Errors

```go
type ValidationError struct {
    ToolName   string
    Parameter  string
    Expected   string
    Received   interface{}
    Suggestion string
}

func (v *Validator) FormatError(err ValidationError) string {
    return fmt.Sprintf(`
**Validation Error in tool call "%s"**
Parameter: %s
Expected: %s
Received: %v
Suggestion: %s
`, err.ToolName, err.Parameter, err.Expected, err.Received, err.Suggestion)
}
```

## Display Formatting

### Tool Result Formatters

```go
type ResultFormatter struct {
    formatters map[string]Formatter
}

func (f *ResultFormatter) FormatResult(toolName string, result interface{}) string {
    formatter, exists := f.formatters[toolName]
    if !exists {
        formatter = f.defaultFormatter
    }
    
    return formatter.Format(result)
}

type Formatter interface {
    Format(interface{}) string
}

// Specific formatters
type FileFormatter struct{}
func (f *FileFormatter) Format(result interface{}) string {
    return fmt.Sprintf("✅ File created: %s", result)
}

type JSONFormatter struct{}
func (f *JSONFormatter) Format(result interface{}) string {
    jsonBytes, _ := json.MarshalIndent(result, "", "  ")
    return string(jsonBytes)
}

type TableFormatter struct{}
func (f *TableFormatter) Format(result interface{}) string {
    // Convert result to table format
    return renderTable(result)
}
```

### Interactive Results

```go
type InteractiveResult struct {
    Content string
    Actions []Action
}

type Action struct {
    Name        string
    Description string
    Handler     func() error
}

func (r *InteractiveResult) Render() string {
    var sb strings.Builder
    
    sb.WriteString(r.Content)
    sb.WriteString("\n\n**Actions:**\n")
    
    for i, action := range r.Actions {
        sb.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, action.Name, action.Description))
    }
    
    return sb.String()
}
```

## Best Practices

### Response Parsing Guidelines

1. **Provider Compatibility**: Handle different response formats uniformly
2. **Error Recovery**: Gracefully handle malformed responses
3. **Streaming Support**: Process partial responses efficiently
4. **Memory Management**: Avoid buffering large responses

### Tool Call Best Practices

1. **Validation First**: Validate tool calls before execution
2. **Timeout Protection**: Always use timeouts for tool execution
3. **Result Size**: Limit result sizes to prevent UI overload
4. **Error Messages**: Provide clear, actionable error messages

### Display Optimization

1. **Lazy Rendering**: Only render visible content
2. **Syntax Highlighting**: Apply appropriate highlighting for different formats
3. **Responsive Design**: Adapt display based on terminal size
4. **Interactive Elements**: Provide interactive elements for complex data

## Integration Examples

### Complete Tool Call Flow

```go
// 1. Parse LLM response
parser := NewResponseParser("openai")
parsed, err := parser.ParseResponse(llmResponse)

// 2. Execute tool calls
executor := NewToolExecutor(registry)
for _, call := range parsed.ToolCalls {
    result, err := executor.ExecuteToolCall(call)
    if err != nil {
        // Handle error
        continue
    }
    
    // 3. Format and display results
    formatter := NewResultFormatter()
    formatted := formatter.FormatResult(call.Name, result)
    
    // 4. Display to user
    display.Render(formatted)
}
```

### Streaming Integration

```go
// Real-time processing
stream := NewStreamingProcessor()
for chunk := range llmStream {
    parsed := stream.ProcessChunk(chunk)
    
    if parsed.HasToolCalls {
        // Handle streaming tool calls
        for _, call := range parsed.ToolCalls {
            go func(tc ToolCall) {
                result := executor.ExecuteToolCall(tc)
                display.UpdateToolResult(tc.ID, result)
            }(call)
        }
    }
    
    // Display content
    display.RenderContent(parsed.Content)
}
```

This comprehensive system enables sophisticated LLM interactions while maintaining a responsive and user-friendly interface.