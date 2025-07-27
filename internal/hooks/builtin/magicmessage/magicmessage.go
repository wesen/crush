package magicmessage

import (
	"context"
	"fmt"
	"math/rand"
	"os"

	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
)

// MagicMessageHook is a built-in hook that injects magic word messages after tool results
type MagicMessageHook struct {
	messageService message.Service
	config         map[string]interface{}
	magicWords     []string
}

// New creates a new magic message hook
func New() *MagicMessageHook {
	return &MagicMessageHook{}
}

// Info returns metadata about this hook
func (m *MagicMessageHook) Info() hooks.HookInfo {
	return hooks.HookInfo{
		Name:        "magic_message_hook",
		Version:     "1.0.0",
		Description: "Appends magic word messages after tool results",
	}
}

// Initialize initializes the hook with configuration and services
func (m *MagicMessageHook) Initialize(config map[string]interface{}, services hooks.ServiceRegistry) error {
	m.config = config
	m.messageService = services.MessageService()

	// Default magic words
	m.magicWords = []string{"ABRACADABRA", "ALAKAZAM", "PRESTO", "HOCUS POCUS", "SESAME"}

	// Allow customization via config
	if config != nil {
		if words, ok := config["magic_words"].([]string); ok {
			m.magicWords = words
		}
	}

	return nil
}

// TransformSession implements TransformSessionHook to get session-level access
func (m *MagicMessageHook) TransformSession(ctx context.Context, sess *session.Session) (hooks.TransformResult[session.Session], error) {
	// This hook doesn't modify the session directly, but needs access for message injection
	return hooks.NoChange[session.Session](), nil
}

// TransformAfterTool implements TransformToolHook for the trigger event
func (m *MagicMessageHook) TransformAfterTool(ctx context.Context, resultCtx *hooks.ToolTransformContext) (hooks.TransformResult[hooks.ToolTransformContext], error) {
	// Only run if environment variable is set for testing
	if os.Getenv("CRUSH_USE_MAGIC_MESSAGE_HOOK") != "1" {
		return hooks.NoChange[hooks.ToolTransformContext](), nil
	}

	// Select random magic word
	magicWord := m.magicWords[rand.Intn(len(m.magicWords))]
	magicText := fmt.Sprintf("HELLO THE MAGIC WORD IS %s", magicWord)

	// Create new message to inject
	_, err := m.messageService.Create(ctx, resultCtx.SessionID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: magicText},
		},
		Model:    "hook-system",
		Provider: "magic-message-hook",
	})
	if err != nil {
		return hooks.TransformResult[hooks.ToolTransformContext]{}, fmt.Errorf("failed to inject magic message: %w", err)
	}

	// Return original context unchanged (non-destructive)
	return hooks.NoChange[hooks.ToolTransformContext](), nil
}
