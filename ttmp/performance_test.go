package main

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/hooks"
)

// BenchmarkSimpleHook tests basic hook performance
func BenchmarkSimpleHook(b *testing.B) {
	hook := &SimpleHook{}
	ctx := context.Background()
	
	callCtx := &hooks.LLMCallCtx{
		SessionID: "test-session",
		AgentID:   "test-agent",
		Model:     "test-model",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hook.BeforeLLMCall(ctx, callCtx)
	}
}

// BenchmarkHookManager tests hook manager performance
func BenchmarkHookManager(b *testing.B) {
	manager := hooks.New()
	hook := &SimpleHook{}
	manager.Add(hook)
	
	ctx := context.Background()
	callCtx := &hooks.LLMCallCtx{
		SessionID: "test-session",
		AgentID:   "test-agent", 
		Model:     "test-model",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.EmitBeforeLLM(ctx, callCtx)
	}
}

type SimpleHook struct{}

func (h *SimpleHook) Info() hooks.HookInfo {
	return hooks.HookInfo{
		Name:        "simple_hook",
		Version:     "1.0.0",
		Description: "Simple hook for performance testing",
	}
}

func (h *SimpleHook) BeforeLLMCall(ctx context.Context, c *hooks.LLMCallCtx) {
	// Minimal processing for baseline performance
	_ = c.SessionID
}
