package graph

import (
	"context"
	"fmt"
	"time"
)

// NodeFunc is the signature for all node logic.
// Read state via View, return a Command with updates and next targets.
type NodeFunc func(ctx context.Context, view View) (*Command, error)

// ErrNamespaceViolation is returned when a node attempts to access or update keys outside its namespace.
var ErrNamespaceViolation = fmt.Errorf("graph: namespace violation")

// RetryPolicy configures automatic retry behavior for node execution.
type RetryPolicy struct {
	MaxAttempts int
	Delay       time.Duration
	MaxDelay    time.Duration
	Multiplier  float64
	Retryable   func(error) bool // Optional: determines if error should trigger retry
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: 3,
		Delay:       100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Multiplier:  2.0,
	}
}

// RetryPolicyBuilder provides a fluent API for constructing retry policies.
//
// Example:
//
//	policy := graph.NewRetryPolicyBuilder().
//	    WithMaxAttempts(5).
//	    WithExponentialBackoff(time.Second, 2.0).
//	    Build()
type RetryPolicyBuilder struct {
	maxAttempts int
	delay       time.Duration
	maxDelay    time.Duration
	multiplier  float64
	retryable   func(error) bool
}

// NewRetryPolicyBuilder creates a new retry policy builder with sensible defaults:
//   - MaxAttempts: 3
//   - Delay: 100ms (base delay)
//   - MaxDelay: 5s
//   - Multiplier: 2.0 (exponential backoff)
func NewRetryPolicyBuilder() *RetryPolicyBuilder {
	return &RetryPolicyBuilder{
		maxAttempts: 3,
		delay:       100 * time.Millisecond,
		maxDelay:    5 * time.Second,
		multiplier:  2.0,
	}
}

// WithMaxAttempts sets the maximum number of execution attempts.
func (b *RetryPolicyBuilder) WithMaxAttempts(n int) *RetryPolicyBuilder {
	b.maxAttempts = n
	return b
}

// WithExponentialBackoff configures exponential backoff.
// Wait time = delay * (multiplier ^ attempt).
//
// Example:
//
//	WithExponentialBackoff(time.Second, 2.0) // 1s, 2s, 4s, 8s, ...
func (b *RetryPolicyBuilder) WithExponentialBackoff(delay time.Duration, multiplier float64) *RetryPolicyBuilder {
	b.delay = delay
	b.multiplier = multiplier
	return b
}

// WithLinearBackoff configures linear backoff.
// Wait time = delay * attempt.
//
// Example:
//
//	WithLinearBackoff(time.Second) // 1s, 2s, 3s, 4s, ...
func (b *RetryPolicyBuilder) WithLinearBackoff(delay time.Duration) *RetryPolicyBuilder {
	b.delay = delay
	b.multiplier = 1.0 // Linear means no exponential growth per attempt
	return b
}

// WithConstantBackoff configures constant delay between retries.
//
// Example:
//
//	WithConstantBackoff(time.Second) // 1s, 1s, 1s, ...
func (b *RetryPolicyBuilder) WithConstantBackoff(delay time.Duration) *RetryPolicyBuilder {
	b.delay = delay
	b.multiplier = 1.0
	b.maxDelay = delay
	return b
}

// WithMaxDelay sets the maximum delay between retries.
func (b *RetryPolicyBuilder) WithMaxDelay(d time.Duration) *RetryPolicyBuilder {
	b.maxDelay = d
	return b
}

// WithRetryableFunc sets a function to determine if an error should trigger retry.
// If not set, all errors are considered retryable.
func (b *RetryPolicyBuilder) WithRetryableFunc(fn func(error) bool) *RetryPolicyBuilder {
	b.retryable = fn
	return b
}

// Build creates the RetryPolicy.
func (b *RetryPolicyBuilder) Build() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: b.maxAttempts,
		Delay:       b.delay,
		MaxDelay:    b.maxDelay,
		Multiplier:  b.multiplier,
		Retryable:   b.retryable,
	}
}

// WithRetry wraps a NodeFunc with retry logic.
// On failure, retries according to the policy with exponential backoff.
//
// Example:
//
//	g.Node("fetch", graph.WithRetry(fetchNode, graph.DefaultRetryPolicy()), "process")
func WithRetry(fn NodeFunc, policy *RetryPolicy) NodeFunc {
	if policy == nil {
		return fn
	}

	return func(ctx context.Context, view View) (*Command, error) {
		var lastErr error
		delay := policy.Delay

		for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
			cmd, err := fn(ctx, view)
			if err == nil {
				return cmd, nil
			}

			lastErr = err

			// Check if error is retryable (if filter is set)
			if policy.Retryable != nil && !policy.Retryable(err) {
				return nil, err // Not retryable, fail immediately
			}

			// Don't sleep after the last attempt
			if attempt < policy.MaxAttempts-1 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}

				// Exponential backoff
				delay = time.Duration(float64(delay) * policy.Multiplier)
				if delay > policy.MaxDelay {
					delay = policy.MaxDelay
				}
			}
		}

		return nil, fmt.Errorf("max retries (%d) exceeded: %w", policy.MaxAttempts, lastErr)
	}
}

// Namespace represents a state namespace for isolation.
type Namespace struct {
	name string
}

// NewNamespace creates a namespace with the given name.
func NewNamespace(name string) Namespace {
	return Namespace{name: name}
}

// Name returns the namespace name.
func (ns Namespace) Name() string {
	return ns.name
}

// Prefix returns a key with the namespace prefix.
func (ns Namespace) Prefix(key string) string {
	return ns.name + "." + key
}

// namespacedView wraps a View to filter keys by namespace.
type namespacedView struct {
	inner         View
	namespace     Namespace
	includeGlobal bool
}

// GetValue implements View interface with namespace filtering.
func (v *namespacedView) GetValue(name string) (any, bool) {
	// Check if key is in allowed namespace
	if !v.isAllowed(name) {
		return nil, false
	}
	return v.inner.GetValue(name)
}

// ManagedValues returns the managed values registry from the inner view.
func (v *namespacedView) ManagedValues() *managedValueRegistry {
	return v.inner.ManagedValues()
}

func (v *namespacedView) isAllowed(key string) bool {
	// Check if key belongs to the namespace (has namespace prefix)
	prefix := v.namespace.name + "."
	if len(key) > len(prefix) && key[:len(prefix)] == prefix {
		return true
	}
	// Allow global keys (no dots) if includeGlobal is true
	if v.includeGlobal {
		for i := 0; i < len(key); i++ {
			if key[i] == '.' {
				return false // Has a dot, so it's namespaced (but not our namespace)
			}
		}
		return true // No dots = global key
	}
	return false
}

// WithNamespace wraps a NodeFunc to filter state access by namespace.
// The wrapped function only sees keys from the specified namespace.
// If includeGlobal is true, global (non-namespaced) keys are also visible.
//
// Example:
//
//	agentNS := graph.NewNamespace("agent1")
//	g.Node("agent1", graph.WithNamespace(agentNode, agentNS, false), "next")
func WithNamespace(fn NodeFunc, ns Namespace, includeGlobal bool) NodeFunc {
	return func(ctx context.Context, view View) (*Command, error) {
		// Wrap view to filter by namespace
		filteredView := &namespacedView{
			inner:         view,
			namespace:     ns,
			includeGlobal: includeGlobal,
		}

		cmd, err := fn(ctx, filteredView)
		if err != nil {
			return cmd, err
		}

		// Validate that updates only contain allowed keys
		if cmd != nil && cmd.Updates != nil {
			for key := range cmd.Updates {
				if !filteredView.isAllowed(key) {
					if includeGlobal {
						return nil, fmt.Errorf("%w: attempted to update key %q (only %s.* and global keys are allowed)",
							ErrNamespaceViolation, key, ns.name)
					}
					return nil, fmt.Errorf("%w: attempted to update key %q (only %s.* keys are allowed)",
						ErrNamespaceViolation, key, ns.name)
				}
			}
		}

		return cmd, nil
	}
}

// Compose combines multiple wrappers around a NodeFunc.
// Wrappers are applied right-to-left (last wrapper runs first).
//
// Example:
//
//	g.Node("fetch", graph.Compose(
//	    fetchNode,
//	    graph.WithRetry(nil, graph.DefaultRetryPolicy()),  // outer: retry
//	    graph.WithNamespace(nil, ns, false),                // inner: namespace
//	), "next")
//
// This is equivalent to: WithRetry(WithNamespace(fetchNode, ns, false), policy)
func Compose(fn NodeFunc, wrappers ...func(NodeFunc) NodeFunc) NodeFunc {
	wrapped := fn
	// Apply in reverse order so first wrapper is outermost
	for i := len(wrappers) - 1; i >= 0; i-- {
		wrapped = wrappers[i](wrapped)
	}
	return wrapped
}
