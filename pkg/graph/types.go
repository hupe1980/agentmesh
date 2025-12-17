package graph

import (
	"context"
	"iter"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
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

// NodeMiddleware wraps node execution.
// This runs for every node during graph execution.
// Output type is fixed to Message for agent workflows.
type NodeMiddleware func(next NodeFunc) NodeFunc

// RunFunc is the function signature for graph execution.
// Input is []message.Message (conversation history), output is Message (response).
type RunFunc func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error]

// RunMiddleware wraps the entire graph execution (Run/Resume).
// Unlike node middleware which runs for every node, run middleware
// intercepts the input before execution starts and the final output after.
// This is useful for:
//   - Input validation/guardrails (check user input once at start)
//   - Output validation/guardrails (check final output once at end)
//   - Logging/observability at the run level
//   - Request/response transformation
type RunMiddleware func(next RunFunc) RunFunc

// Executor runs the graph.
type Executor interface {
	Run(ctx context.Context, cfg *ExecutorConfig, input []message.Message, opts ...runOption) iter.Seq2[message.Message, error]
}

// ExecutorNode represents a node for the executor.
// Output type is fixed to Message.
type ExecutorNode struct {
	Name    string
	Fn      NodeFunc
	Targets []string
}

// ExecutionConfig holds node execution configuration.
// This includes the graph structure, entry points, middleware, and output settings.
// Types are fixed for Message-based agent workflows.
type ExecutionConfig struct {
	// Nodes contains all graph nodes indexed by name.
	Nodes map[string]ExecutorNode

	// EntryPoints are the starting nodes for execution.
	EntryPoints []string

	// NodeMiddleware wraps node execution.
	// This runs for every node during graph execution.
	NodeMiddleware []NodeMiddleware

	// Store provides state storage.
	Store Store

	// KeyRegistry holds type-erased reducers for state merging.
	KeyRegistry KeyRegistry
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
// Types are fixed for Message-based agent workflows.
type ExecutorConfig struct {
	// Execution contains node and execution settings.
	Execution ExecutionConfig

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
type InputMapper func(ctx context.Context, scope ReadOnlyScope) ([]message.Message, error)

// OutputMapper maps subgraph output to parent graph state updates.
// Used with Subgraph to transform child graph output into state updates for the parent.
type OutputMapper func(ctx context.Context, output message.Message) (Updates, error)

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
