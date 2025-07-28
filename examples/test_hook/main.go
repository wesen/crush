package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/charmbracelet/crush/hooks"
)

// TestHook is a simple test hook that demonstrates the plugin system
type TestHook struct {
	messageService hooks.MessageService
	config         map[string]interface{}
	logger         *slog.Logger
}

// NewHook is the required entry point function that plugins must export
func NewHook() hooks.Hook {
	return &TestHook{
		logger: slog.Default(),
	}
}

// Info returns metadata about this hook
func (t *TestHook) Info() hooks.HookInfo {
	return hooks.HookInfo{
		Name:        "test_plugin_hook",
		Version:     "1.0.0",
		Description: "A test hook plugin that demonstrates the plugin loading system",
	}
}

// Initialize initializes the hook with configuration and services
func (t *TestHook) Initialize(config map[string]interface{}, services hooks.ServiceRegistry) error {
	t.config = config
	t.messageService = services.MessageService()
	t.logger.Info("Test plugin hook initialized", "config", config)
	return nil
}

// TransformAfterTool implements TransformToolHook interface
func (t *TestHook) TransformAfterTool(ctx context.Context, resultCtx *hooks.ToolTransformContext) (hooks.TransformResult[hooks.ToolTransformContext], error) {
	// Only run if environment variable is set for testing
	if os.Getenv("CRUSH_USE_TEST_PLUGIN_HOOK") != "1" {
		return hooks.NoChange[hooks.ToolTransformContext](), nil
	}

	t.logger.Info("Test plugin hook processing tool result",
		"session_id", resultCtx.SessionID,
		"tool_name", resultCtx.ToolName)

	// Create a test message to inject
	testMessage := "🔌 Test plugin hook was here!"

	// Create new message to inject
	_, err := t.messageService.Create(ctx, resultCtx.SessionID, hooks.CreateMessageParams{
		Role: hooks.Assistant,
		Parts: []hooks.ContentPart{
			hooks.TextContent{Text: testMessage},
		},
		Model:    "plugin-system",
		Provider: "test-plugin-hook",
	})
	if err != nil {
		return hooks.TransformResult[hooks.ToolTransformContext]{}, fmt.Errorf("failed to inject test message: %w", err)
	}

	t.logger.Info("Test plugin hook injected message", "message", testMessage)

	// Return original context unchanged (non-destructive)
	return hooks.NoChange[hooks.ToolTransformContext](), nil
}

// Ensure the hook is built as a plugin (not a regular executable)
func main() {
	// This should not be called when used as a plugin
	panic("This is a plugin, not a standalone executable")
}
