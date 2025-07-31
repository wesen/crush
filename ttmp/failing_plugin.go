package main

import (
	"context"
	"fmt"

	"github.com/charmbracelet/crush/hooks"
)

// FailingPlugin demonstrates circuit breaker functionality
type FailingPlugin struct {
	callCount int
}

// Info returns metadata about this plugin
func (p *FailingPlugin) Info() hooks.HookInfo {
	return hooks.HookInfo{
		Name:        "failing_plugin",
		Version:     "1.0.0",
		Description: "Plugin that deliberately fails to test circuit breaker",
	}
}

// Initialize sets up the plugin
func (p *FailingPlugin) Initialize(config map[string]interface{}, services hooks.ServiceRegistry) error {
	return nil
}

// BeforeLLMCall deliberately fails to trigger circuit breaker
func (p *FailingPlugin) BeforeLLMCall(ctx context.Context, c *hooks.LLMCallCtx) {
	p.callCount++
	panic(fmt.Sprintf("deliberate failure #%d", p.callCount))
}

// NewHook is the plugin initializer function
func NewHook() hooks.Hook {
	return &FailingPlugin{}
}
