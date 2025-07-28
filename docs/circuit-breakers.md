# Hook Circuit Breakers

The Crush hook system includes a comprehensive circuit breaker implementation that provides safety and reliability for hook execution. Circuit breakers protect against cascading failures by temporarily disabling failing hooks and providing automatic recovery mechanisms.

## Features

- **Per-hook circuit breakers** with individual state tracking
- **Configurable failure thresholds** and recovery periods  
- **Timeout protection** for hook execution
- **Automatic bypass** when circuits are open
- **Half-open state** for testing recovery
- **Comprehensive metrics** collection and health reporting
- **Manual reset** capability for operational control

## Circuit States

### Closed (Normal Operation)
- All hook calls are allowed through
- Failures are tracked but don't block execution
- Circuit opens when failure threshold is reached

### Open (Protection Mode)
- All hook calls are immediately rejected
- No actual hook execution occurs
- Prevents cascading failures
- Automatically transitions to Half-Open after recovery timeout

### Half-Open (Testing Recovery)
- Limited number of test calls allowed
- Success closes the circuit back to normal operation
- Any failure immediately reopens the circuit

## Configuration

Circuit breakers can be configured at two levels:

### Default Configuration
Applied to all hooks unless overridden:

```json
{
  "hooks": {
    "circuit_breaker": {
      "failure_threshold": 3,
      "timeout": 5000,
      "recovery_timeout": 30000,
      "half_open_max_calls": 3,
      "success_threshold": 2
    }
  }
}
```

### Per-Hook Configuration
Override settings for specific hooks:

```json
{
  "hooks": {
    "per_hook_config": {
      "critical-hook": {
        "failure_threshold": 2,
        "timeout": 3000,
        "recovery_timeout": 15000,
        "half_open_max_calls": 2,
        "success_threshold": 1
      }
    }
  }
}
```

## Configuration Parameters

| Parameter | Description | Default | Unit |
|-----------|-------------|---------|------|
| `failure_threshold` | Number of consecutive failures before opening circuit | 3 | count |
| `timeout` | Maximum time to wait for hook execution | 5000 | milliseconds |
| `recovery_timeout` | Wait time before transitioning Open → Half-Open | 30000 | milliseconds |
| `half_open_max_calls` | Number of test calls allowed in Half-Open state | 3 | count |
| `success_threshold` | Successful calls needed to close circuit from Half-Open | 2 | count |

## Metrics and Monitoring

Circuit breakers collect comprehensive metrics for monitoring:

```go
type CircuitBreakerMetrics struct {
    TotalCalls       int64 // Total number of execution attempts
    SuccessfulCalls  int64 // Successfully completed calls
    FailedCalls      int64 // Calls that returned errors
    TimeoutCalls     int64 // Calls that exceeded timeout
    CircuitOpenCalls int64 // Calls rejected due to open circuit
    StateChanges     int64 // Number of state transitions
}
```

### Accessing Metrics

Get metrics for all hooks:
```go
metrics := hookManager.GetCircuitBreakerMetrics()
for hookName, metric := range metrics {
    fmt.Printf("Hook %s: %d total calls, %d failures\n", 
        hookName, metric.TotalCalls, metric.FailedCalls)
}
```

Get health status:
```go
status := hookManager.GetHookHealthStatus()
for hookName, healthy := range status {
    fmt.Printf("Hook %s is healthy: %v\n", hookName, healthy)
}
```

## Logging

Circuit breaker state changes are automatically logged:

- **Debug level**: All circuit breaker operations and metrics
- **Info level**: State transitions (Open → Half-Open → Closed)
- **Warn level**: Circuit opening due to failures

Example log output:
```
INFO Circuit breaker half-open hook=transform-hook previous_state=open max_test_calls=3
WARN Circuit breaker opened hook=critical-hook previous_state=closed consecutive_failures=2 recovery_timeout=15000ms
INFO Circuit breaker closed hook=transform-hook previous_state=half-open consecutive_failures=0
```

## Operational Control

### Manual Reset
Force a circuit breaker back to closed state:

```go
// Reset all circuit breakers
hookManager.ResetCircuitBreakers()

// Reset specific hook
success := hookManager.ResetHookCircuitBreaker("problematic-hook")
```

### Health Checks
Circuit breakers can be integrated into health check endpoints:

```go
func healthCheck() map[string]interface{} {
    return map[string]interface{}{
        "hook_health": hookManager.GetHookHealthStatus(),
        "hook_metrics": hookManager.GetCircuitBreakerMetrics(),
    }
}
```

## Best Practices

### Configuration Guidelines

1. **Failure Threshold**: Set based on hook criticality
   - Critical hooks: 1-2 failures
   - Normal hooks: 3-5 failures
   - Non-critical hooks: 5-10 failures

2. **Timeout**: Based on expected execution time
   - Fast hooks: 1-3 seconds
   - Normal hooks: 5-10 seconds
   - Heavy hooks: 15-30 seconds

3. **Recovery Timeout**: Allow time for underlying issues to resolve
   - Transient issues: 30-60 seconds
   - Infrastructure issues: 2-5 minutes
   - Dependent service issues: 5-15 minutes

### Monitoring

1. **Set up alerts** for circuit breaker state changes
2. **Monitor failure rates** and adjust thresholds accordingly
3. **Track recovery patterns** to optimize recovery timeouts
4. **Use metrics** to identify problematic hooks

### Development

1. **Test with circuit breakers** enabled in development
2. **Verify timeout handling** for all hooks
3. **Implement proper error handling** in hooks
4. **Log meaningful error messages** for debugging

## Integration Examples

### Custom Hook with Circuit Breaker Awareness

```go
type ResilientHook struct {
    name string
}

func (h *ResilientHook) Info() HookInfo {
    return HookInfo{Name: h.name, Version: "1.0.0"}
}

func (h *ResilientHook) BeforeLLMCall(ctx context.Context, c *LLMCallCtx) {
    // Hook implementation that handles errors gracefully
    if err := h.doWork(ctx); err != nil {
        // Log error but don't panic - let circuit breaker handle it
        slog.Error("Hook work failed", "hook", h.name, "error", err)
        return
    }
}

func (h *ResilientHook) doWork(ctx context.Context) error {
    // Respect context cancellation for timeout handling
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Do actual work
        return nil
    }
}
```

### Configuration Loading

```go
// Load configuration and create managers with circuit breaker support
config, err := config.Load("config.json")
if err != nil {
    log.Fatal(err)
}

var hooksMgr *hooks.Manager
if config.Hooks != nil && config.Hooks.CircuitBreaker != nil {
    defaultConfig, perHookConfig := convertCircuitBreakerConfig(config)
    hooksMgr = hooks.NewWithCircuitBreakerConfig(defaultConfig, perHookConfig)
} else {
    hooksMgr = hooks.New()
}
```

## Troubleshooting

### Common Issues

1. **Circuit frequently opening**
   - Lower failure threshold or increase timeout
   - Check hook implementation for reliability issues
   - Monitor upstream dependencies

2. **Circuit not opening when expected**
   - Verify failure threshold configuration
   - Check if hooks are properly returning errors
   - Ensure timeouts are appropriate

3. **Slow recovery**
   - Reduce recovery timeout
   - Lower success threshold for faster validation
   - Increase half-open max calls for better testing

### Debugging

Enable debug logging to see detailed circuit breaker operations:

```bash
CRUSH_LOG_LEVEL=debug ./crush
```

This will show all circuit breaker decisions and state changes for detailed analysis.
