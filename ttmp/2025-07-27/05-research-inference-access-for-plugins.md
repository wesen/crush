# Inference System Access for Plugins Research

## Executive Summary

Research into enabling **hooks, tools, and widgets** to perform LLM inference by accessing Crush's configured providers with existing credentials. This document analyzes current provider architecture, credential management, and proposes minimal API surface for plugin inference access.

## Table of Contents

1. [Current Provider Architecture](#current-provider-architecture)
2. [Credential Resolution System](#credential-resolution-system)
3. [Provider Creation Flow](#provider-creation-flow)
4. [Plugin Inference Interface Proposals](#plugin-inference-interface-proposals)
5. [Implementation Approaches](#implementation-approaches)
6. [Usage Examples](#usage-examples)

---

## 1. Current Provider Architecture

### Core Provider Interface (`internal/llm/provider/provider.go`)

```go
type Provider interface {
    SendMessages(ctx, messages, tools) (*ProviderResponse, error)
    StreamResponse(ctx, messages, tools) <-chan ProviderEvent
    Model() catwalk.Model
}
```

### Provider Creation (`provider.NewProvider()`)

```pseudo
# Current provider instantiation
providerCfg := config.GetProviderForModel(modelType)
provider, err := provider.NewProvider(providerCfg, options...)

# Supports 7 provider types
- Anthropic (Claude)
- OpenAI (GPT) 
- Gemini (Google)
- Bedrock (AWS)
- Azure OpenAI
- VertexAI (Google Cloud)
- XAI (Grok)
```

### Agent Provider Management (`internal/llm/agent/agent.go`)

Agents maintain **3 provider instances**:
- `provider`: Main inference (agent's configured model)
- `titleProvider`: Title generation (small model)  
- `summarizeProvider`: Summarization (small model)

```go
type agent struct {
    provider          provider.Provider
    titleProvider     provider.Provider  
    summarizeProvider provider.Provider
    // ...
}
```

## 2. Credential Resolution System

### Configuration Resolution (`internal/config/resolve.go`)

Credentials are resolved through:
- **Environment variables**: `$API_KEY`, `${VAR}`
- **Command substitution**: `$(command)`
- **Shell expansion**: Full shell-style variable substitution

```go
type VariableResolver interface {
    ResolveValue(value string) (string, error)
}

// Example usage in provider creation
resolvedAPIKey, err := config.Get().Resolve(cfg.APIKey)
```

### Provider Configuration (`internal/config/config.go`)

```go
type ProviderConfig struct {
    ID           string
    BaseURL      string
    APIKey       string            // Resolved at provider creation
    Type         catwalk.Type      // anthropic, openai, etc.
    ExtraHeaders map[string]string // Also resolved
    ExtraBody    map[string]any
    Models       []catwalk.Model
}
```

**Key insight**: Credentials are resolved **once** during provider instantiation, not stored in resolved form.

## 3. Provider Creation Flow

### Current Flow

```pseudo
# Agent creation (agent.go:109)
providerCfg := config.GetProviderForModel(agentCfg.Model)
opts := []ProviderClientOption{
    WithModel(agentCfg.Model),
    WithSystemMessage(prompt),
}
provider, err := provider.NewProvider(*providerCfg, opts...)
```

### Provider Factory Function (`provider.NewProvider()`)

```go
func NewProvider(cfg ProviderConfig, opts ...ProviderClientOption) (Provider, error) {
    // Resolve credentials
    resolvedAPIKey, err := config.Get().Resolve(cfg.APIKey)
    
    // Resolve headers  
    resolvedExtraHeaders := make(map[string]string)
    for key, value := range cfg.ExtraHeaders {
        resolvedValue, err := config.Get().Resolve(value)
        resolvedExtraHeaders[key] = resolvedValue
    }
    
    // Create provider-specific client
    switch cfg.Type {
    case catwalk.TypeAnthropic:
        return &baseProvider[AnthropicClient]{...}, nil
    case catwalk.TypeOpenAI:
        return &baseProvider[OpenAIClient]{...}, nil
    // ... other types
    }
}
```

**Key insight**: Factory function handles credential resolution + client creation in one step.

## 4. Plugin Inference Interface Proposals

### Option A: Provider Factory Access

Expose the provider factory to plugins with credential resolution:

```pseudo
# Plugin receives factory function
type PluginInferenceFactory interface {
    CreateProvider(modelType, systemPrompt) (Provider, error)
    CreateProviderFromConfig(providerID, systemPrompt) (Provider, error)
    ListAvailableModels() []ModelInfo
}

# Usage in plugin
func (p *MyPlugin) Run(ctx, call, session) (ToolResponse, TransformResult, error) {
    provider, err := p.inferenceFactory.CreateProvider("small", "You are helpful")
    if err != nil { return err }
    
    resp, err := provider.SendMessages(ctx, messages, nil)
    // ... use inference result
}
```

### Option B: Inference Service Interface

Higher-level abstraction over provider details:

```pseudo
type PluginInferenceService interface {
    SendMessages(modelType, systemPrompt, messages) (*InferenceResponse, error)
    StreamMessages(modelType, systemPrompt, messages) <-chan InferenceEvent
    GetModelInfo(modelType) (ModelInfo, error)
}

# Simplified usage
func (p *MyPlugin) Run(ctx, call, session) (ToolResponse, TransformResult, error) {
    resp, err := p.inference.SendMessages("small", "Be helpful", messages)
    return ToolResponse{Content: resp.Content}, TransformResult{}, nil
}
```

### Option C: Agent Provider Access

Direct access to agent's existing providers:

```pseudo
type PluginContext struct {
    SessionID     string
    MainProvider  Provider  # agent's main model
    SmallProvider Provider  # agent's small model  
    // ... other context
}

# Plugin receives pre-configured providers
func (p *MyPlugin) Run(ctx, call, session, pluginCtx) (ToolResponse, TransformResult, error) {
    resp, err := pluginCtx.SmallProvider.SendMessages(ctx, messages, nil)
    // ... 
}
```

## 5. Implementation Approaches

### Approach 1: Extend Hook/Tool Context

Add inference capability to existing plugin contexts:

```pseudo
# Enhanced hook context
type HookContext struct {
    SessionID  string
    MessageID  string  
    AgentID    string
    Metadata   map[string]any
    Inference  PluginInferenceService  # NEW
}

# Enhanced tool plugin interface  
type Tool interface {
    Info() ToolInfo
    Run(ctx, call, session, inference PluginInferenceService) (ToolResponse, TransformResult, error)
}
```

### Approach 2: Plugin-Specific Factory

Plugins receive factory during initialization:

```pseudo
# Plugin initialization
type Plugin interface {
    Initialize(config, inferenceFactory PluginInferenceFactory) error
    // ...
}

# Plugin stores factory for later use
type MyPlugin struct {
    config    map[string]any
    inference PluginInferenceFactory
}
```

### Approach 3: Service Injection

Registry injects inference service:

```pseudo
# Registry provides services to plugins
type PluginRegistry interface {
    LoadPlugin(path, config, services PluginServices) error
}

type PluginServices struct {
    Inference PluginInferenceService
    // ... other services
}
```

## 6. Usage Examples

### Example 1: Summarization Tool

```pseudo
type SummarizeTool struct {
    inference PluginInferenceService
}

func (s *SummarizeTool) Run(ctx, call, session, inference) (ToolResponse, TransformResult, error) {
    # Extract parameters
    maxLength := call.Input.max_length
    
    # Build summarization prompt
    prompt := fmt.Sprintf("Summarize in %d words", maxLength)
    messages := []Message{{Role: "user", Content: session.getFullText()}}
    
    # Use small model for efficiency
    resp, err := inference.SendMessages("small", prompt, messages)
    if err != nil { return ToolResponse{}, TransformResult{}, err }
    
    # Return summary and optionally modify session
    summary := resp.Content
    newSession := session.withSummary(summary)
    
    return ToolResponse{Content: summary}, 
           TransformResult{Modified: &newSession}, 
           nil
}
```

### Example 2: Code Review Hook

```pseudo
type CodeReviewHook struct {
    inference PluginInferenceService
}

func (c *CodeReviewHook) AfterToolResult(ctx, resultCtx) error {
    # Only review code changes
    if resultCtx.ToolName != "edit" && resultCtx.ToolName != "write" {
        return nil
    }
    
    # Extract file changes  
    fileContent := resultCtx.Result.Content
    
    # Get code review
    reviewPrompt := "Briefly review this code for issues"
    messages := []Message{{Role: "user", Content: fileContent}}
    
    resp, err := c.inference.SendMessages("small", reviewPrompt, messages)
    if err != nil { return err }
    
    # Add review as metadata (displayed in UI)
    resultCtx.Metadata["code_review"] = resp.Content
    return nil
}
```

### Example 3: Dynamic Prompt Generator

```pseudo
type SmartPromptWidget struct {
    inference PluginInferenceService
}

func (s *SmartPromptWidget) GeneratePrompt(userInput, context) (string, error) {
    # Analyze user intent and context to suggest better prompt
    metaPrompt := `Given this user input and context, suggest a more effective prompt:
    Input: %s
    Context: %s
    
    Respond with just the improved prompt.`
    
    prompt := fmt.Sprintf(metaPrompt, userInput, context)
    messages := []Message{{Role: "user", Content: prompt}}
    
    resp, err := s.inference.SendMessages("small", "", messages)
    if err != nil { return userInput, err }  # fallback to original
    
    return resp.Content, nil
}
```

---

## Minimal MVP Implementation

**Recommended approach**: **Option A** (Provider Factory) with **Approach 1** (Context Extension)

### Core Interface

```pseudo
type PluginInferenceFactory interface {
    CreateProvider(modelType SelectedModelType, systemPrompt string) (Provider, error)
    GetAvailableModelTypes() []SelectedModelType
}

# Implementation reuses existing provider.NewProvider() 
type pluginInferenceFactory struct {
    config *Config
}

func (p *pluginInferenceFactory) CreateProvider(modelType SelectedModelType, systemPrompt string) (Provider, error) {
    providerCfg := config.GetProviderForModel(modelType)
    if providerCfg == nil {
        return nil, fmt.Errorf("no provider for model type %s", modelType)
    }
    
    opts := []ProviderClientOption{
        WithModel(modelType),
        WithSystemMessage(systemPrompt),
    }
    
    return provider.NewProvider(*providerCfg, opts...)
}
```

### Context Integration

```pseudo
# Add to existing hook context
type ToolCallContext struct {
    HookContext
    ToolCall tools.ToolCall
    ToolName string
    Inference PluginInferenceFactory  # NEW
}

# Add to plugin tool interface
type Tool interface {
    Info() ToolInfo
    Run(ctx, call, session) (ToolResponse, TransformResult, error)
    
    # Optional: tools can implement this to receive inference access
    WithInference(PluginInferenceFactory) Tool
}
```

This approach **reuses existing provider creation logic**, inherits credential resolution, and provides minimal API surface while enabling powerful inference capabilities for plugins. 