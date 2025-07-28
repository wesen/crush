package hooks

import (
	"context"
	"testing"
)

// BenchmarkSimpleHook tests basic hook performance
func BenchmarkSimpleHook(b *testing.B) {
	hook := &simpleHook{}
	ctx := context.Background()
	
	callCtx := &LLMCallCtx{
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
	manager := New()
	hook := &simpleHook{}
	manager.Add(hook)
	
	ctx := context.Background()
	callCtx := &LLMCallCtx{
		SessionID: "test-session",
		AgentID:   "test-agent", 
		Model:     "test-model",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.EmitBeforeLLM(ctx, callCtx)
	}
}

type simpleHook struct{}

func (h *simpleHook) Info() HookInfo {
	return HookInfo{
		Name:        "simple_hook",
		Version:     "1.0.0",
		Description: "Simple hook for performance testing",
	}
}

func (h *simpleHook) BeforeLLMCall(ctx context.Context, c *LLMCallCtx) {
	// Minimal processing for baseline performance
	_ = c.SessionID
}
