package main

import (
	"context"
	"log/slog"

	"github.com/charmbracelet/crush/hooks"
)

// TestPlugin is a simple test plugin for demonstrating the hook system
type TestPlugin struct {
	logger *slog.Logger
}

// Info returns metadata about this plugin
func (p *TestPlugin) Info() hooks.HookInfo {
	return hooks.HookInfo{
		Name:        "test_plugin",
		Version:     "1.0.0",
		Description: "Simple test plugin for end-to-end testing",
	}
}

// Initialize sets up the plugin
func (p *TestPlugin) Initialize(config map[string]interface{}, services hooks.ServiceRegistry) error {
	p.logger = slog.Default()
	p.logger.Info("Test plugin initialized", "name", "test_plugin")
	return nil
}

// BeforeLLMCall logs before LLM calls
func (p *TestPlugin) BeforeLLMCall(ctx context.Context, c *hooks.LLMCallCtx) {
	p.logger.Info("test_plugin: before LLM call",
		"session_id", c.SessionID,
		"model", c.Model)
}

// AfterLLMInference logs after LLM inference
func (p *TestPlugin) AfterLLMInference(ctx context.Context, r *hooks.LLMRespCtx) {
	p.logger.Info("test_plugin: after LLM inference",
		"session_id", r.SessionID,
		"duration", r.Duration)
}

// BeforeToolCall logs before tool calls
func (p *TestPlugin) BeforeToolCall(ctx context.Context, c *hooks.ToolCallCtx) {
	p.logger.Info("test_plugin: before tool call",
		"session_id", c.SessionID,
		"tool_name", c.ToolName)
}

// AfterToolResult logs after tool results
func (p *TestPlugin) AfterToolResult(ctx context.Context, r *hooks.ToolResCtx) {
	p.logger.Info("test_plugin: after tool result",
		"session_id", r.SessionID,
		"tool_name", r.ToolName,
		"duration", r.Duration,
		"error", r.Err)
}

// NewHook is the plugin initializer function that gets called by the plugin loader
func NewHook() hooks.Hook {
	return &TestPlugin{}
}
