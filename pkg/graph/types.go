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
type Middleware func(next NodeFunc) NodeFunc

// Executor runs the graph.
type Executor[I, O any] interface {
	Run(ctx context.Context, cfg *ExecutorConfig[I, O], input I, opts ...RunOption) iter.Seq2[O, error]
}

// ExecutorConfig provides the executor with graph configuration.
type ExecutorConfig[I, O any] struct {
	Nodes            map[string]ExecutorNode
	EntryPoints      []string
	InterruptsBefore map[string]*interruptConfig
	InterruptsAfter  map[string]*interruptConfig
	Middleware       []Middleware
	Store            Store
	Checkpointer     checkpoint.Checkpointer
	RunID            string
	OutputKey        string // Name of the key that produces outputs
	OutputIsList     bool   // True if output key is a ListKey (yield items individually)
}

// ExecutorNode represents a node for the executor.
type ExecutorNode struct {
	Name    string
	Fn      NodeFunc
	Targets []string
}

// InterruptOption configures an interrupt.
type InterruptOption func(*interruptConfig)

type interruptConfig struct {
	guard              ApprovalGuard
	feedbackAnnotation bool
}

// ApprovalGuard determines if approval is needed.
type ApprovalGuard func(ctx context.Context, view View) (needsApproval bool, reason string, err error)

// InputMapper maps parent graph state to subgraph input.
// Used with Subgraph to transform parent state into the input type expected by the child graph.
type InputMapper[SI any] func(ctx context.Context, view View) (SI, error)

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
