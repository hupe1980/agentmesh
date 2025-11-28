package graph

import (
	"errors"
)

// NodeConfig holds execution policies for a node.
// Policies are applied by the executor when running the node.
type NodeConfig struct {
	RetryPolicy *RetryPolicy
}

// NodeOption configures a node's execution policies.
type NodeOption func(*NodeConfig)

// WithRetryPolicy sets the retry policy for a node.
//
// Example:
//
//	g.AddNode("api_call", apiNode,
//	    WithRetryPolicy(&RetryPolicy{
//	        MaxAttempts: 3,
//	        Backoff: ExponentialBackoff(100 * time.Millisecond),
//	    }))
func WithRetryPolicy(policy *RetryPolicy) NodeOption {
	return func(c *NodeConfig) {
		c.RetryPolicy = policy
	}
}

// defaultNodeConfig returns a NodeConfig with sensible defaults.
func defaultNodeConfig() *NodeConfig {
	return &NodeConfig{
		RetryPolicy: &RetryPolicy{
			MaxAttempts: 1, // No retries by default
			Backoff:     nil,
			Retryable:   nil, // All errors retryable
		},
	}
}

// Validate checks if the node configuration is valid.
func (c *NodeConfig) Validate() error {
	if c.RetryPolicy != nil {
		if c.RetryPolicy.MaxAttempts < 1 {
			return errors.New("graph: retry policy MaxAttempts must be >= 1")
		}
	}

	return nil
}
