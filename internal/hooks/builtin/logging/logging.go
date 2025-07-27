package logging

import (
	"context"
	"log/slog"
	"os"

	"github.com/charmbracelet/crush/internal/hooks"
)

// LoggingHook is a built-in hook that logs all events using structured logging.
type LoggingHook struct {
	logger *slog.Logger
}

// New creates a new logging hook that writes to a file.
func New(defaultLogger *slog.Logger) *LoggingHook {
	// Create or append to hooks.log file
	file, err := os.OpenFile("hooks.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// Fallback to default logger if file creation fails
		return &LoggingHook{logger: defaultLogger}
	}

	// Create a new logger that writes to the file
	fileLogger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	return &LoggingHook{
		logger: fileLogger,
	}
}

// Info returns metadata about this hook.
func (l *LoggingHook) Info() hooks.HookInfo {
	return hooks.HookInfo{
		Name:        "logging_hook",
		Version:     "1.0.0",
		Description: "Logs LLM calls and tool executions with structured logging",
	}
}

// BeforeLLMCall logs before an LLM call is made.
func (l *LoggingHook) BeforeLLMCall(ctx context.Context, c *hooks.LLMCallCtx) {
	l.logger.Info("before_llm_call",
		slog.String("session_id", c.SessionID),
		slog.String("agent_id", c.AgentID),
		slog.String("model", c.Model),
	)
}

// AfterLLMInference logs after an LLM inference is completed.
func (l *LoggingHook) AfterLLMInference(ctx context.Context, r *hooks.LLMRespCtx) {
	l.logger.Info("after_llm_inference",
		slog.String("session_id", r.SessionID),
		slog.String("agent_id", r.AgentID),
		slog.Duration("duration", r.Duration),
	)
}

// BeforeToolCall logs before a tool call is made.
func (l *LoggingHook) BeforeToolCall(ctx context.Context, c *hooks.ToolCallCtx) {
	l.logger.Info("before_tool_call",
		slog.String("session_id", c.SessionID),
		slog.String("agent_id", c.AgentID),
		slog.String("tool_name", c.ToolName),
	)
}

// AfterToolResult logs after a tool result is received.
func (l *LoggingHook) AfterToolResult(ctx context.Context, r *hooks.ToolResCtx) {
	l.logger.Info("after_tool_result",
		slog.String("session_id", r.SessionID),
		slog.String("agent_id", r.AgentID),
		slog.String("tool_name", r.ToolName),
		slog.Duration("duration", r.Duration),
		slog.Any("error", r.Err),
	)
}
