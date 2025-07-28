package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// LogCapture captures log output for testing
type LogCapture struct {
	buffer *bytes.Buffer
	logger *slog.Logger
}

// NewLogCapture creates a new log capture instance
func NewLogCapture() *LogCapture {
	buffer := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)
	
	return &LogCapture{
		buffer: buffer,
		logger: logger,
	}
}

// GetLogs returns all captured logs as a slice of log entries
func (lc *LogCapture) GetLogs() []map[string]interface{} {
	lines := strings.Split(strings.TrimSpace(lc.buffer.String()), "\n")
	var logs []map[string]interface{}
	
	for _, line := range lines {
		if line == "" {
			continue
		}
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
			logs = append(logs, logEntry)
		}
	}
	
	return logs
}

// GetLogsContaining returns logs that contain the specified message substring
func (lc *LogCapture) GetLogsContaining(message string) []map[string]interface{} {
	var matchingLogs []map[string]interface{}
	for _, log := range lc.GetLogs() {
		if msg, ok := log["msg"].(string); ok && strings.Contains(msg, message) {
			matchingLogs = append(matchingLogs, log)
		}
	}
	return matchingLogs
}

// SimpleTestHook for basic testing
type SimpleTestHook struct {
	name        string
	version     string
	description string
}

func NewSimpleTestHook(name, version, description string) *SimpleTestHook {
	return &SimpleTestHook{
		name:        name,
		version:     version,
		description: description,
	}
}

func (h *SimpleTestHook) Info() HookInfo {
	return HookInfo{
		Name:        h.name,
		Version:     h.version,
		Description: h.description,
	}
}

func (h *SimpleTestHook) BeforeLLMCall(ctx context.Context, c *LLMCallCtx) {
	// Simple implementation for testing
}

func TestBasicHookManagerLogging(t *testing.T) {
	// Capture logs
	logCapture := NewLogCapture()
	
	// Set the default logger to our capture logger
	originalLogger := slog.Default()
	slog.SetDefault(logCapture.logger)
	defer slog.SetDefault(originalLogger)
	
	// Create manager
	manager := New()
	
	// Add test hook
	hook := NewSimpleTestHook("test-hook", "1.0.0", "Test hook for logging")
	manager.Add(hook)
	
	// Check that hook addition was logged
	addLogs := logCapture.GetLogsContaining("Hook added to event manager")
	if len(addLogs) != 1 {
		t.Errorf("Expected 1 hook addition log, got %d", len(addLogs))
	}
	
	// Test hook execution logging
	ctx := context.Background()
	manager.EmitBeforeLLM(ctx, &LLMCallCtx{})
	
	// Check that hook execution was logged
	execLogs := logCapture.GetLogsContaining("Emitting BeforeLLM event")
	if len(execLogs) != 1 {
		t.Errorf("Expected 1 BeforeLLM emission log, got %d", len(execLogs))
	}
	
	// Verify we have debug logs
	allLogs := logCapture.GetLogs()
	var debugCount int
	for _, log := range allLogs {
		if level, ok := log["level"].(string); ok && level == "DEBUG" {
			debugCount++
		}
	}
	
	if debugCount == 0 {
		t.Error("Expected some DEBUG level logs")
	}
}

func TestPluginLoaderBasicLogging(t *testing.T) {
	// Capture logs
	logCapture := NewLogCapture()
	
	// Set the default logger to our capture logger
	originalLogger := slog.Default()
	slog.SetDefault(logCapture.logger)
	defer slog.SetDefault(originalLogger)
	
	// Create plugin loader
	loader := NewPluginLoader(logCapture.logger)
	
	// Test plugin discovery with non-existent directory
	paths, err := loader.DiscoverPlugins([]string{"non-existent-dir"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(paths) != 0 {
		t.Errorf("Expected 0 plugins, got %d", len(paths))
	}
	
	// Check discovery logging
	discoveryLogs := logCapture.GetLogsContaining("Plugin discovery completed")
	if len(discoveryLogs) != 1 {
		t.Errorf("Expected 1 plugin discovery log, got %d", len(discoveryLogs))
	}
	
	// Check directory skip logging
	skipLogs := logCapture.GetLogsContaining("Plugin directory does not exist, skipping")
	if len(skipLogs) != 1 {
		t.Errorf("Expected 1 directory skip log, got %d", len(skipLogs))
	}
}

func TestCircuitBreakerBasicLogging(t *testing.T) {
	// Capture logs
	logCapture := NewLogCapture()
	
	// Set the default logger to our capture logger
	originalLogger := slog.Default()
	slog.SetDefault(logCapture.logger)
	defer slog.SetDefault(originalLogger)
	
	// Create circuit breaker manager
	config := CircuitBreakerConfig{
		FailureThreshold:     2,
		Timeout:              1000,
		RecoveryTimeout:      5000,
		HalfOpenMaxCalls:     2,
		SuccessThreshold:     1,
	}
	cbManager := NewCircuitBreakerManager(config)
	
	// Get circuit breaker (this should create one)
	cb := cbManager.GetCircuitBreaker("test-hook")
	
	// Check creation logging
	creationLogs := logCapture.GetLogsContaining("Circuit breaker created for hook")
	if len(creationLogs) != 1 {
		t.Errorf("Expected 1 circuit breaker creation log, got %d", len(creationLogs))
	}
	
	// Test successful execution
	ctx := context.Background()
	err := cb.Execute(ctx, func() error { return nil })
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Check that operation was logged
	allLogs := logCapture.GetLogs()
	found := false
	for _, log := range allLogs {
		if msg, ok := log["msg"].(string); ok && strings.Contains(msg, "Circuit breaker operation") {
			found = true
			break
		}
	}
	
	if !found {
		t.Error("Expected circuit breaker operation log")
	}
}

func TestLogStructureAndTiming(t *testing.T) {
	// Capture logs
	logCapture := NewLogCapture()
	
	// Set the default logger to our capture logger
	originalLogger := slog.Default()
	slog.SetDefault(logCapture.logger)
	defer slog.SetDefault(originalLogger)
	
	// Create and use hook system
	manager := New()
	hook := NewSimpleTestHook("timing-test-hook", "1.0.0", "Hook for testing timing")
	manager.Add(hook)
	
	// Execute hook
	ctx := context.Background()
	manager.EmitBeforeLLM(ctx, &LLMCallCtx{})
	
	// Get all logs
	allLogs := logCapture.GetLogs()
	
	// Verify log structure
	for _, log := range allLogs {
		// Check required fields
		if _, ok := log["time"]; !ok {
			t.Error("Log entry missing 'time' field")
		}
		if _, ok := log["level"]; !ok {
			t.Error("Log entry missing 'level' field")
		}
		if _, ok := log["msg"]; !ok {
			t.Error("Log entry missing 'msg' field")
		}
		
		// Check for timing measurements
		if _, hasDuration := log["duration"]; hasDuration {
			// Verify it's a valid duration string
			if dur, ok := log["duration"].(string); ok {
				if !strings.Contains(dur, "ms") && !strings.Contains(dur, "µs") && !strings.Contains(dur, "s") && !strings.Contains(dur, "ns") {
					t.Errorf("Invalid duration format: %v", dur)
				}
			}
		}
	}
	
	// Check that we have different log levels
	var debugCount, infoCount int
	for _, log := range allLogs {
		if level, ok := log["level"].(string); ok {
			switch level {
			case "DEBUG":
				debugCount++
			case "INFO":
				infoCount++
			}
		}
	}
	
	if debugCount == 0 {
		t.Error("Expected some DEBUG level logs")
	}
	if infoCount == 0 {
		t.Error("Expected some INFO level logs")
	}
}

func TestPerformanceMetricsInLogs(t *testing.T) {
	// Capture logs
	logCapture := NewLogCapture()
	
	// Set the default logger to our capture logger
	originalLogger := slog.Default()
	slog.SetDefault(logCapture.logger)
	defer slog.SetDefault(originalLogger)
	
	// Create manager with hook
	manager := New()
	hook := NewSimpleTestHook("perf-test-hook", "1.0.0", "Performance test hook")
	manager.Add(hook)
	
	// Execute hook multiple times to generate metrics
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		manager.EmitBeforeLLM(ctx, &LLMCallCtx{})
		time.Sleep(1 * time.Millisecond) // Small delay to ensure timing
	}
	
	// Get metrics
	metrics := manager.GetCircuitBreakerMetrics()
	_ = metrics
	
	// Check for timing-related logs
	allLogs := logCapture.GetLogs()
	var foundDuration bool
	var foundMetrics bool
	
	for _, log := range allLogs {
		if _, hasDuration := log["duration"]; hasDuration {
			foundDuration = true
		}
		if strings.Contains(fmt.Sprintf("%v", log), "total_calls") {
			foundMetrics = true
		}
	}
	
	if !foundDuration {
		t.Error("Expected to find duration measurements in logs")
	}
	if !foundMetrics {
		t.Error("Expected to find metrics information in logs") 
	}
}

