# Crush Hook System - Comprehensive Logging Implementation

## Overview

This implementation adds comprehensive logging and instrumentation to the entire Crush hook system, building upon the existing plugin loading and circuit breaker implementations.

## Key Features Implemented

### 1. **Structured Logging with slog**
- Uses Go's structured logging (`log/slog`) throughout the hook system
- Consistent JSON format for all log entries
- Configurable log levels (Debug, Info, Warn, Error)
- Component-based logging with consistent field names

### 2. **Debug Logging Support**
- Integrates with existing `.crush/logs` directory structure
- Respects debug flags from configuration
- Verbose logging for development and troubleshooting
- Performance-optimized logging that doesn't impact production

### 3. **Performance Metrics & Timing**
- Detailed timing measurements for all hook operations
- Plugin loading duration tracking (file stat, open, symbol lookup, instantiation)
- Hook execution timing with circuit breaker state monitoring
- Event emission performance tracking

### 4. **Hook Lifecycle Logging**
- Hook registration with interface analysis
- Plugin discovery with detailed file scanning logs
- Hook initialization timing and status
- Manager creation with configuration details

### 5. **Circuit Breaker Instrumentation**
- State change logging (Closed → Open → Half-Open transitions)
- Detailed metrics logging (success rate, call counts, failures)
- Configuration tracking (per-hook vs default settings)
- Health status monitoring

### 6. **Error Tracking & Recovery**
- Comprehensive error logging with context
- Panic recovery with stack trace information
- Failed plugin loading with detailed error reasons
- Hook execution failures with circuit breaker context

## Files Modified/Enhanced

### Core Hook System Files
- **`internal/hooks/manager.go`** - Enhanced event manager with comprehensive logging
- **`internal/hooks/plugin_manager.go`** - Plugin lifecycle logging and performance tracking
- **`internal/hooks/transform_manager.go`** - Transform operation logging and instrumentation
- **`internal/hooks/circuit_breaker.go`** - Circuit breaker state and metrics logging
- **`internal/hooks/plugin_loader.go`** - Plugin discovery and loading with detailed file operations

### Test Files Added
- **`internal/hooks/simple_logging_test.go`** - Comprehensive test suite for logging functionality

## Logging Categories

### 1. **DEBUG Level Logs**
- Hook interface analysis and registration steps
- Plugin file discovery and scanning details
- Circuit breaker operation details
- Event emission eligibility analysis
- Performance timing for all operations

### 2. **INFO Level Logs**
- Successful hook registrations
- Plugin loading completions
- Manager creation confirmations
- Circuit breaker state changes
- Hook execution summaries

### 3. **WARN Level Logs**
- Circuit breaker openings
- Hook execution failures
- Plugin loading issues (non-fatal)
- Interface implementation warnings

### 4. **ERROR Level Logs**
- Plugin loading failures
- Hook initialization failures
- Critical circuit breaker failures
- Panic recoveries with stack traces

## Log Structure Examples

### Hook Registration
```json
{
  "time": "2025-07-28T10:32:53.581314476-04:00",
  "level": "INFO",
  "msg": "Hook registration completed successfully",
  "component": "plugin_manager",
  "name": "demo-hook-1",
  "version": "1.0.0",
  "description": "First demo hook",
  "transform_hook": false,
  "event_hook": true,
  "implemented_interfaces": ["BeforeLLMCaller", "AfterLLMInferencer"],
  "registration_duration": "45.2µs"
}
```

### Circuit Breaker State Change
```json
{
  "time": "2025-07-28T10:32:53.581338058-04:00",
  "level": "DEBUG",
  "msg": "Circuit breaker operation",
  "hook": "demo-hook-1",
  "state": "closed",
  "reason": "success",
  "consecutive_failures": 0,
  "total_calls": 1,
  "successful_calls": 1,
  "failed_calls": 0,
  "timeout_calls": 0,
  "circuit_open_calls": 0,
  "state_changes": 0,
  "success_rate": 100.0
}
```

### Hook Execution
```json
{
  "time": "2025-07-28T10:32:53.581347601-04:00",
  "level": "DEBUG",
  "msg": "Hook execution completed successfully",
  "component": "hook_manager",
  "hook": "demo-hook-1",
  "duration": "39.477µs",
  "cb_state": "closed"
}
```

## Performance Considerations

1. **Conditional Logging**: Debug logs are only generated when debug level is enabled
2. **Structured Fields**: Uses slog's efficient field logging rather than string concatenation
3. **Timing Measurements**: Minimal overhead timing using `time.Since()`
4. **Component Isolation**: Each manager has its own logger instance for filtering

## Integration with Existing System

1. **Debug Flag Support**: Integrates with existing `crush -d` and config `"debug": true`
2. **Log Directory**: Uses existing `.crush/logs/` directory structure
3. **Circuit Breaker**: Builds upon existing circuit breaker implementation
4. **Plugin System**: Enhances existing plugin loading without breaking changes

## Testing

The implementation includes comprehensive tests that verify:
- Log structure and required fields
- Performance timing measurements
- Different log levels (DEBUG, INFO, WARN, ERROR)
- Component-based filtering
- Circuit breaker logging integration
- Plugin discovery and loading logs

Run tests with:
```bash
go test -run "TestBasic|TestCircuitBreaker|TestLog" -v ./internal/hooks/
```

## Usage Examples

### Enable Debug Logging
```bash
# Via CLI flag
crush -d

# Via configuration
{
  "options": {
    "debug": true
  }
}
```

### View Hook Logs
```bash
# View all logs
crush logs

# Follow logs in real-time
crush logs -f

# View only hook-related logs (if using component filtering)
crush logs | grep '"component":"hook_manager"'
```

## Benefits

1. **Observability**: Complete visibility into hook system operations
2. **Debugging**: Detailed timing and state information for troubleshooting
3. **Performance Monitoring**: Track hook execution performance and circuit breaker health
4. **Production Safety**: Structured logging that doesn't impact performance
5. **Maintainability**: Consistent logging patterns across the entire hook system

This implementation makes the entire Crush hook system fully observable and debuggable while maintaining high performance and production readiness.
