// Package policies provides ready-to-use callback implementations for common
// operational patterns like rate limiting, circuit breakers, retries, and more.
//
// These policies are built on top of the agentmesh callback system and use
// closure-based state management for thread-safe operation. They can be easily
// composed together to create sophisticated execution policies.
//
// # Rate Limiting
//
// RateLimit and PerNodeRateLimit implement sliding window rate limiting:
//
//	// Global rate limit: 100 requests per minute
//	manager.RegisterBeforeModel(policies.RateLimit(100, time.Minute))
//
//	// Per-node rate limit: 10 requests per second for specific node
//	manager.RegisterBeforeModel(policies.PerNodeRateLimit("expensive_api", 10, time.Second))
//
// # Circuit Breakers
//
// CircuitBreaker and PerNodeCircuitBreaker implement the circuit breaker pattern
// with three states: Closed (normal), Open (failing), and Half-Open (testing recovery):
//
//	config := policies.DefaultCircuitBreakerConfig()
//	config.MaxFailures = 5
//	config.Timeout = 30 * time.Second
//
//	before, after, onError := policies.CircuitBreaker(config)
//	manager.RegisterBeforeModel(before)
//	manager.RegisterAfterModel(after)
//	manager.RegisterOnModelError(onError)
//
// # Retry Policies
//
// Multiple retry strategies with exponential backoff:
//
//	// Simple exponential backoff
//	config := policies.DefaultRetryConfig()
//	manager.RegisterOnModelError(policies.ExponentialBackoffRetry(config))
//
//	// Retry with overall timeout
//	manager.RegisterOnModelError(policies.RetryWithTimeout(config, 5*time.Minute))
//
//	// Conditional retry for specific errors
//	shouldRetry := func(err error) bool {
//	    return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrTimeout)
//	}
//	manager.RegisterOnModelError(policies.ConditionalRetry(config, shouldRetry))
//
// # Composing Policies
//
// Multiple policies can be composed together. They execute in registration order:
//
//	manager := callbacks.NewManager()
//
//	// 1. Rate limit first (prevent overload)
//	manager.RegisterBeforeModel(policies.RateLimit(100, time.Minute))
//
//	// 2. Circuit breaker (detect failures)
//	cbBefore, cbAfter, cbError := policies.CircuitBreaker(policies.DefaultCircuitBreakerConfig())
//	manager.RegisterBeforeModel(cbBefore)
//	manager.RegisterAfterModel(cbAfter)
//	manager.RegisterOnModelError(cbError)
//
//	// 3. Retry on errors (with backoff)
//	retryConfig := policies.DefaultRetryConfig()
//	retryConfig.MaxAttempts = 3
//	manager.RegisterOnModelError(policies.ExponentialBackoffRetry(retryConfig))
//
// This creates a robust execution policy: rate limiting prevents overload,
// circuit breaker detects systemic failures, and retry handles transient errors.
//
// # State Management
//
// All policies use closure-based state management, making them:
//   - Thread-safe (using sync.Mutex)
//   - Independent of graph state
//   - Self-contained and composable
//   - Zero-configuration for basic usage
//
// # Custom Policies
//
// You can create custom policies following the same patterns:
//
//	func CustomPolicy() callbacks.BeforeModelCallback {
//	    // Closure state
//	    state := &myState{}
//
//	    return func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
//	        // Access graph state via s
//	        messages := s.MessagesSnapshot()
//
//	        // Implement policy logic
//	        // ...
//
//	        // Return nil to continue, or a message to short-circuit
//	        return nil, nil
//	    }
//	}
package policies
