package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
)

const (
	// DefaultRetryMaxAttempts is the default maximum number of retry attempts
	// for node execution. After this many failures, the error propagates.
	//
	// Why 3? Industry standard for transient failures (network timeouts, rate limits).
	// First attempt + 2 retries = 3 total attempts, sufficient for most transient issues.
	DefaultRetryMaxAttempts = 3

	// DefaultRetryDelay is the default base delay between retry attempts.
	// Combined with exponential backoff (2.0 multiplier), delays are:
	// 100ms -> 200ms -> 400ms -> ... (capped at DefaultRetryMaxDelay)
	DefaultRetryDelay = 100 * time.Millisecond

	// DefaultRetryMaxDelay is the maximum delay between retry attempts.
	// Prevents excessive wait times during extended outages.
	//
	// Why 5s? Balances patience for recovery with user experience.
	// Longer delays rarely help—if service is down >5s, manual intervention needed.
	DefaultRetryMaxDelay = 5 * time.Second

	// DefaultRetryMultiplier is the exponential backoff multiplier.
	// Each retry delay = previous delay * multiplier (until MaxDelay).
	//
	// Why 2.0? Standard exponential backoff. Quickly backs off to reduce
	// load on failing services while not being overly aggressive.
	DefaultRetryMultiplier = 2.0
)

// NodeFunc is the typed signature for node logic.
// Output type is fixed to Message for agent workflows.
// Read state via Scope, optionally stream partial outputs, return a Command.
//
// Example:
//
//	func myNode(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
//	    messages := scope.Messages()
//	    scope.Stream(partialMessage)  // Stream partial output
//	    return graph.Reply(finalMessage).End()
//	}
type NodeFunc func(ctx context.Context, scope Scope) (*Command, error)

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
// Uses DefaultRetryMaxAttempts (3), DefaultRetryDelay (100ms),
// DefaultRetryMaxDelay (5s), and DefaultRetryMultiplier (2.0).
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: DefaultRetryMaxAttempts,
		Delay:       DefaultRetryDelay,
		MaxDelay:    DefaultRetryMaxDelay,
		Multiplier:  DefaultRetryMultiplier,
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
//   - MaxAttempts: DefaultRetryMaxAttempts (3)
//   - Delay: DefaultRetryDelay (100ms base delay)
//   - MaxDelay: DefaultRetryMaxDelay (5s)
//   - Multiplier: DefaultRetryMultiplier (2.0 exponential backoff)
func NewRetryPolicyBuilder() *RetryPolicyBuilder {
	return &RetryPolicyBuilder{
		maxAttempts: DefaultRetryMaxAttempts,
		delay:       DefaultRetryDelay,
		maxDelay:    DefaultRetryMaxDelay,
		multiplier:  DefaultRetryMultiplier,
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

	return func(ctx context.Context, scope Scope) (*Command, error) {
		var lastErr error
		delay := policy.Delay

		for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
			cmd, err := fn(ctx, scope)
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

// namespacedScope wraps a Scope to filter keys by namespace.
type namespacedScope struct {
	inner         Scope
	namespace     Namespace
	includeGlobal bool
}

func (s *namespacedScope) GetValue(name string) (any, bool) {
	if !s.isAllowed(name) {
		return nil, false
	}
	return s.inner.GetValue(name)
}

func (s *namespacedScope) ManagedValues() *ManagedValueRegistry {
	return s.inner.ManagedValues()
}

func (s *namespacedScope) ToMap() map[string]any {
	innerMap := s.inner.ToMap()
	result := make(map[string]any)
	for k, val := range innerMap {
		if s.isAllowed(k) {
			result[k] = val
		}
	}
	return result
}

func (s *namespacedScope) Stream(value message.Message) {
	s.inner.Stream(value)
}

func (s *namespacedScope) NodeName() string {
	return s.inner.NodeName()
}

func (s *namespacedScope) Messages() []message.Message {
	return s.inner.Messages()
}

func (s *namespacedScope) LastMessage() message.Message {
	return s.inner.LastMessage()
}

func (s *namespacedScope) isAllowed(key string) bool {
	prefix := s.namespace.name + "."
	if len(key) > len(prefix) && key[:len(prefix)] == prefix {
		return true
	}
	if s.includeGlobal {
		for i := 0; i < len(key); i++ {
			if key[i] == '.' {
				return false
			}
		}
		return true
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
	return func(ctx context.Context, scope Scope) (*Command, error) {
		// Wrap scope to filter by namespace
		filteredScope := &namespacedScope{
			inner:         scope,
			namespace:     ns,
			includeGlobal: includeGlobal,
		}

		cmd, err := fn(ctx, filteredScope)
		if err != nil {
			return cmd, err
		}

		// Validate that updates only contain allowed keys
		if cmd != nil && cmd.Updates != nil {
			for key := range cmd.Updates {
				if !filteredScope.isAllowed(key) {
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
//	    func(fn graph.NodeFunc) graph.NodeFunc { return graph.WithRetry(fn, policy) },
//	    func(fn graph.NodeFunc) graph.NodeFunc { return graph.WithNamespace(fn, ns, false) },
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
