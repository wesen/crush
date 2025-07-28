// Package hooks provides the public interface for the Crush hook system.
// This package re-exports types from internal/hooks to allow plugin development.
package hooks

import (
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/llm/tools"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
)

// Re-export core hook types and interfaces
type (
	Hook                = hooks.Hook
	HookInfo            = hooks.HookInfo
	HookInitializer     = hooks.HookInitializer
	ServiceRegistry     = hooks.ServiceRegistry
	
	// Event hook interfaces
	BeforeLLMCaller      = hooks.BeforeLLMCaller
	AfterLLMInferencer   = hooks.AfterLLMInferencer
	BeforeToolCaller     = hooks.BeforeToolCaller
	AfterToolResulter    = hooks.AfterToolResulter
	
	// Transform hook interfaces
	TransformToolHook    = hooks.TransformToolHook
	TransformSessionHook = hooks.TransformSessionHook
	
	// Context types
	LLMCallCtx           = hooks.LLMCallCtx
	LLMRespCtx           = hooks.LLMRespCtx
	ToolCallCtx          = hooks.ToolCallCtx
	ToolResCtx           = hooks.ToolResCtx
	SessionHookContext   = hooks.SessionHookContext
	ToolTransformContext = hooks.ToolTransformContext
	
	// Transform result type
	TransformResult[T any] = hooks.TransformResult[T]
)

// Re-export convenience constructors with proper type instantiation
func Modified[T any](value T) TransformResult[T] {
	return hooks.Modified(value)
}

func Skip[T any]() TransformResult[T] {
	return hooks.Skip[T]()
}

func NoChange[T any]() TransformResult[T] {
	return hooks.NoChange[T]()
}

func WithMetadata[T any](key string, value any) TransformResult[T] {
	return hooks.WithMetadata[T](key, value)
}

// Re-export message and session types that plugins commonly need
type (
	Message            = message.Message
	MessageService     = message.Service
	CreateMessageParams = message.CreateMessageParams
	ContentPart        = message.ContentPart
	TextContent        = message.TextContent
	Role               = message.MessageRole
	
	Session            = session.Session
	SessionService     = session.Service
	
	ToolCall           = tools.ToolCall
	ToolResponse       = tools.ToolResponse
)

// Re-export message roles
const (
	User      = message.User
	Assistant = message.Assistant
	System    = message.System
)
