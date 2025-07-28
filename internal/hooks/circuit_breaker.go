package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CircuitBreakerState represents the current state of a circuit breaker
type CircuitBreakerState int

const (
	// Closed means the circuit is closed and operations are allowed
	Closed CircuitBreakerState = iota
	// Open means the circuit is open and operations are rejected
	Open
	// HalfOpen means the circuit is allowing limited operations to test recovery
	HalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for circuit breaker behavior
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit
	FailureThreshold int `json:"failure_threshold"`
	// Timeout is the maximum time to wait for hook execution (in milliseconds)
	Timeout int `json:"timeout"`
	// RecoveryTimeout is how long to wait before transitioning from Open to HalfOpen (in milliseconds)
	RecoveryTimeout int `json:"recovery_timeout"`
	// HalfOpenMaxCalls is the number of calls allowed in HalfOpen state before deciding to close or reopen
	HalfOpenMaxCalls int `json:"half_open_max_calls"`
	// SuccessThreshold is the number of successful calls needed in HalfOpen to close the circuit
	SuccessThreshold int `json:"success_threshold"`
}

// DefaultCircuitBreakerConfig returns sensible defaults for circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:     3,               // Open after 3 consecutive failures
		Timeout:              5000,            // 5 second timeout for hook execution
		RecoveryTimeout:      30000,           // 30 seconds before trying recovery
		HalfOpenMaxCalls:     3,               // Allow 3 calls to test recovery
		SuccessThreshold:     2,               // Need 2 successes to consider recovered
	}
}

// CircuitBreakerMetrics holds metrics for monitoring circuit breaker behavior
type CircuitBreakerMetrics struct {
	TotalCalls       int64 `json:"total_calls"`
	SuccessfulCalls  int64 `json:"successful_calls"`
	FailedCalls      int64 `json:"failed_calls"`
	TimeoutCalls     int64 `json:"timeout_calls"`
	CircuitOpenCalls int64 `json:"circuit_open_calls"`
	StateChanges     int64 `json:"state_changes"`
}

// CircuitBreaker implements circuit breaker pattern for hook execution
type CircuitBreaker struct {
	hookName string
	config   CircuitBreakerConfig
	
	mu                sync.RWMutex
	state             CircuitBreakerState
	failureCount      int
	consecutiveFailures int
	halfOpenCalls     int
	halfOpenSuccesses int
	lastFailureTime   time.Time
	metrics           CircuitBreakerMetrics
}

// NewCircuitBreaker creates a new circuit breaker for a hook
func NewCircuitBreaker(hookName string, config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		hookName: hookName,
		config:   config,
		state:    Closed,
		metrics:  CircuitBreakerMetrics{},
	}
}

// Execute wraps hook execution with circuit breaker logic
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	// Check if circuit should allow the call
	if !cb.allowCall() {
		cb.recordCircuitOpen()
		return fmt.Errorf("circuit breaker open for hook %s", cb.hookName)
	}

	// Create context with timeout
	timeoutDuration := time.Duration(cb.config.Timeout) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	// Execute with timeout protection
	errChan := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("hook panic: %v", r)
			}
		}()
		errChan <- fn()
	}()

	select {
	case err := <-errChan:
		if err != nil {
			cb.recordFailure(err)
			return err
		}
		cb.recordSuccess()
		return nil
	case <-ctx.Done():
		cb.recordTimeout()
		return fmt.Errorf("hook %s timed out after %v", cb.hookName, timeoutDuration)
	}
}

// allowCall determines if a call should be allowed based on circuit state
func (cb *CircuitBreaker) allowCall() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.metrics.TotalCalls++

	switch cb.state {
	case Closed:
		return true
	case Open:
		// Check if recovery timeout has passed
		if time.Since(cb.lastFailureTime) >= time.Duration(cb.config.RecoveryTimeout)*time.Millisecond {
			cb.transitionToHalfOpen()
			return true
		}
		return false
	case HalfOpen:
		// Allow limited calls to test recovery
		if cb.halfOpenCalls < cb.config.HalfOpenMaxCalls {
			cb.halfOpenCalls++
			return true
		}
		return false
	default:
		return false
	}
}

// recordSuccess records a successful hook execution
func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.metrics.SuccessfulCalls++
	cb.consecutiveFailures = 0

	if cb.state == HalfOpen {
		cb.halfOpenSuccesses++
		if cb.halfOpenSuccesses >= cb.config.SuccessThreshold {
			cb.transitionToClosed()
		}
	}

	cb.logStateIfChanged("success")
}

// recordFailure records a failed hook execution
func (cb *CircuitBreaker) recordFailure(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.metrics.FailedCalls++
	cb.consecutiveFailures++
	cb.lastFailureTime = time.Now()

	if cb.state == Closed && cb.consecutiveFailures >= cb.config.FailureThreshold {
		cb.transitionToOpen()
	} else if cb.state == HalfOpen {
		// Any failure in half-open state immediately reopens the circuit
		cb.transitionToOpen()
	}

	cb.logStateIfChanged(fmt.Sprintf("failure: %v", err))
}

// recordTimeout records a timeout during hook execution
func (cb *CircuitBreaker) recordTimeout() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.metrics.TimeoutCalls++
	cb.consecutiveFailures++
	cb.lastFailureTime = time.Now()

	if cb.state == Closed && cb.consecutiveFailures >= cb.config.FailureThreshold {
		cb.transitionToOpen()
	} else if cb.state == HalfOpen {
		cb.transitionToOpen()
	}

	cb.logStateIfChanged("timeout")
}

// recordCircuitOpen records when a call was rejected due to open circuit
func (cb *CircuitBreaker) recordCircuitOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.metrics.CircuitOpenCalls++
}

// transitionToClosed transitions the circuit to closed state
func (cb *CircuitBreaker) transitionToClosed() {
	oldState := cb.state
	cb.state = Closed
	cb.halfOpenCalls = 0
	cb.halfOpenSuccesses = 0
	cb.consecutiveFailures = 0
	
	if oldState != Closed {
		cb.metrics.StateChanges++
		slog.Info("Circuit breaker closed",
			"hook", cb.hookName,
			"previous_state", oldState.String(),
			"consecutive_failures", cb.consecutiveFailures)
	}
}

// transitionToOpen transitions the circuit to open state
func (cb *CircuitBreaker) transitionToOpen() {
	oldState := cb.state
	cb.state = Open
	cb.halfOpenCalls = 0
	cb.halfOpenSuccesses = 0
	
	if oldState != Open {
		cb.metrics.StateChanges++
		slog.Warn("Circuit breaker opened",
			"hook", cb.hookName,
			"previous_state", oldState.String(),
			"consecutive_failures", cb.consecutiveFailures,
			"recovery_timeout", fmt.Sprintf("%dms", cb.config.RecoveryTimeout))
	}
}

// transitionToHalfOpen transitions the circuit to half-open state
func (cb *CircuitBreaker) transitionToHalfOpen() {
	oldState := cb.state
	cb.state = HalfOpen
	cb.halfOpenCalls = 0
	cb.halfOpenSuccesses = 0
	
	if oldState != HalfOpen {
		cb.metrics.StateChanges++
		slog.Info("Circuit breaker half-open",
			"hook", cb.hookName,
			"previous_state", oldState.String(),
			"max_test_calls", cb.config.HalfOpenMaxCalls)
	}
}

// logStateIfChanged logs state changes for monitoring
func (cb *CircuitBreaker) logStateIfChanged(reason string) {
	slog.Debug("Circuit breaker operation",
		"hook", cb.hookName,
		"state", cb.state.String(),
		"reason", reason,
		"consecutive_failures", cb.consecutiveFailures,
		"total_calls", cb.metrics.TotalCalls,
		"successful_calls", cb.metrics.SuccessfulCalls,
		"failed_calls", cb.metrics.FailedCalls,
		"timeout_calls", cb.metrics.TimeoutCalls,
		"circuit_open_calls", cb.metrics.CircuitOpenCalls,
		"state_changes", cb.metrics.StateChanges,
		"success_rate", func() float64 {
			if cb.metrics.TotalCalls == 0 {
				return 0
			}
			return float64(cb.metrics.SuccessfulCalls) / float64(cb.metrics.TotalCalls) * 100
		}())
}

// State returns the current circuit breaker state (thread-safe)
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Metrics returns a copy of current metrics (thread-safe)
func (cb *CircuitBreaker) Metrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.metrics
}

// IsHealthy returns true if the circuit breaker is allowing calls
func (cb *CircuitBreaker) IsHealthy() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == Closed || cb.state == HalfOpen
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	slog.Info("Circuit breaker manually reset", "hook", cb.hookName)
	cb.transitionToClosed()
}

// CircuitBreakerManager manages circuit breakers for all hooks
type CircuitBreakerManager struct {
	mu               sync.RWMutex
	circuitBreakers  map[string]*CircuitBreaker
	defaultConfig    CircuitBreakerConfig
	perHookConfig    map[string]CircuitBreakerConfig
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(defaultConfig CircuitBreakerConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		circuitBreakers: make(map[string]*CircuitBreaker),
		defaultConfig:   defaultConfig,
		perHookConfig:   make(map[string]CircuitBreakerConfig),
	}
}

// GetCircuitBreaker gets or creates a circuit breaker for a hook
func (cbm *CircuitBreakerManager) GetCircuitBreaker(hookName string) *CircuitBreaker {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	if cb, exists := cbm.circuitBreakers[hookName]; exists {
		slog.Debug("Retrieved existing circuit breaker",
			"hook", hookName,
			"state", cb.State().String(),
			"total_circuit_breakers", len(cbm.circuitBreakers))
		return cb
	}

	// Use per-hook config if available, otherwise use default
	config := cbm.defaultConfig
	configSource := "default"
	if hookConfig, exists := cbm.perHookConfig[hookName]; exists {
		config = hookConfig
		configSource = "per-hook"
	}

	slog.Debug("Creating new circuit breaker for hook",
		"hook", hookName,
		"config_source", configSource,
		"config", config,
		"existing_circuit_breakers", len(cbm.circuitBreakers))

	cb := NewCircuitBreaker(hookName, config)
	cbm.circuitBreakers[hookName] = cb

	slog.Info("Circuit breaker created for hook",
		"hook", hookName,
		"config_source", configSource,
		"failure_threshold", config.FailureThreshold,
		"timeout_ms", config.Timeout,
		"recovery_timeout_ms", config.RecoveryTimeout,
		"total_circuit_breakers", len(cbm.circuitBreakers))

	return cb
}

// SetHookConfig sets specific configuration for a hook
func (cbm *CircuitBreakerManager) SetHookConfig(hookName string, config CircuitBreakerConfig) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	oldConfig, hadConfig := cbm.perHookConfig[hookName]
	cbm.perHookConfig[hookName] = config
	
	slog.Debug("Set hook-specific circuit breaker config",
		"hook", hookName,
		"new_config", config,
		"had_previous_config", hadConfig,
		"previous_config", func() CircuitBreakerConfig {
			if hadConfig {
				return oldConfig
			}
			return CircuitBreakerConfig{}
		}())
	
	// Update existing circuit breaker if it exists
	if cb, exists := cbm.circuitBreakers[hookName]; exists {
		cb.mu.Lock()
		oldCbConfig := cb.config
		cb.config = config
		cb.mu.Unlock()
		
		slog.Info("Updated existing circuit breaker configuration",
			"hook", hookName,
			"old_config", oldCbConfig,
			"new_config", config,
			"current_state", cb.State().String())
	}
}

// GetAllMetrics returns metrics for all circuit breakers
func (cbm *CircuitBreakerManager) GetAllMetrics() map[string]CircuitBreakerMetrics {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	metrics := make(map[string]CircuitBreakerMetrics)
	for hookName, cb := range cbm.circuitBreakers {
		metrics[hookName] = cb.Metrics()
	}
	return metrics
}

// GetHealthStatus returns health status for all hooks
func (cbm *CircuitBreakerManager) GetHealthStatus() map[string]bool {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	status := make(map[string]bool)
	for hookName, cb := range cbm.circuitBreakers {
		status[hookName] = cb.IsHealthy()
	}
	return status
}

// ResetAll resets all circuit breakers
func (cbm *CircuitBreakerManager) ResetAll() {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	for _, cb := range cbm.circuitBreakers {
		cb.Reset()
	}
}

// ResetHook resets a specific hook's circuit breaker
func (cbm *CircuitBreakerManager) ResetHook(hookName string) bool {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	if cb, exists := cbm.circuitBreakers[hookName]; exists {
		cb.Reset()
		return true
	}
	return false
}
