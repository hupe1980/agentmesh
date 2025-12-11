package graph

import (
	"context"
	"iter"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// ApprovalResponse represents a human approval decision.
type ApprovalResponse struct {
	Decision    ApprovalDecision
	Reason      string
	User        string
	Timestamp   time.Time
	Edits       Updates
	Annotations map[string]any
}

// ApprovalDecision represents the approval outcome.
type ApprovalDecision string

// ApprovalDecision constants.
const (
	ApprovalApproved ApprovalDecision = "approved"
	ApprovalRejected ApprovalDecision = "rejected"
)

// Middleware wraps node execution.
// The type parameter O matches the graph's output type.
type Middleware[O any] func(next NodeFunc[O]) NodeFunc[O]

// Executor runs the graph.
type Executor[I, O any] interface {
	Run(ctx context.Context, cfg *ExecutorConfig[I, O], input I, opts ...RunOption) iter.Seq2[O, error]
}

// ExecutorNode represents a node for the executor.
// The type parameter O matches the graph's output type.
type ExecutorNode[O any] struct {
	Name    string
	Fn      NodeFunc[O]
	Targets []string
}

// ExecutionConfig holds node execution configuration.
// This includes the graph structure, entry points, middleware, and output settings.
// The type parameter O matches the graph's output type.
type ExecutionConfig[O any] struct {
	// Nodes contains all graph nodes indexed by name.
	Nodes map[string]ExecutorNode[O]

	// EntryPoints are the starting nodes for execution.
	EntryPoints []string

	// Middleware wraps node execution.
	Middleware []Middleware[O]

	// Store provides state storage.
	Store Store

	// OutputKey is the name of the key that produces outputs.
	OutputKey string

	// OutputIsList is true if output key is a ListKey (yield items individually).
	OutputIsList bool
}

// CheckpointConfig holds checkpointing configuration.
// This enables state persistence, fault tolerance, and resume capabilities.
type CheckpointConfig struct {
	// Checkpointer handles state persistence.
	Checkpointer checkpoint.Checkpointer

	// RunID identifies this execution run for checkpointing.
	RunID string
}

// InterruptConfig holds interrupt configuration for human-in-the-loop workflows.
// Interrupts pause execution to await human approval before or after specific nodes.
type InterruptConfig struct {
	// Before maps node names to interrupt configs that trigger before node execution.
	Before map[string]*interruptConfig

	// After maps node names to interrupt configs that trigger after node execution.
	After map[string]*interruptConfig
}

// ExecutorConfig provides the executor with graph configuration.
// It composes focused configuration structs for better separation of concerns.
type ExecutorConfig[I, O any] struct {
	// Execution contains node and execution settings.
	Execution ExecutionConfig[O]

	// Checkpoint contains checkpointing settings.
	Checkpoint CheckpointConfig

	// Interrupt contains interrupt settings for human-in-the-loop workflows.
	Interrupt InterruptConfig
}

// InterruptOption configures an interrupt.
type InterruptOption func(*interruptConfig)

type interruptConfig struct {
	guard              ApprovalGuard
	feedbackAnnotation bool
}

// ApprovalGuard determines if approval is needed.
// Uses ReadOnlyScope (not Scope) since it doesn't need streaming access.
type ApprovalGuard func(ctx context.Context, scope ReadOnlyScope) (needsApproval bool, reason string, err error)

// InputMapper maps parent graph state to subgraph input.
// Used with Subgraph to transform parent state into the input type expected by the child graph.
// Uses ReadOnlyScope (not Scope) since it's a read-only operation.
type InputMapper[SI any] func(ctx context.Context, scope ReadOnlyScope) (SI, error)

// OutputMapper maps subgraph output to parent graph state updates.
// Used with Subgraph to transform child graph output into state updates for the parent.
type OutputMapper[SO any] func(ctx context.Context, output SO) (Updates, error)

// WithApprovalGuard sets a guard function for the interrupt.
func WithApprovalGuard(guard ApprovalGuard) InterruptOption {
	return func(cfg *interruptConfig) {
		cfg.guard = guard
	}
}

// WithFeedbackAnnotation enables recording approval in message history.
func WithFeedbackAnnotation(enabled bool) InterruptOption {
	return func(cfg *interruptConfig) {
		cfg.feedbackAnnotation = enabled
	}
}
