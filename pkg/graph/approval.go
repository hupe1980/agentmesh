package graph

import (
	"context"
	"time"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// ApprovalDecision represents the type of approval decision made by a human reviewer.
type ApprovalDecision string

const (
	// ApprovalApproved indicates the action was approved without modifications
	ApprovalApproved ApprovalDecision = "APPROVED"

	// ApprovalRejected indicates the action was rejected and should not proceed
	ApprovalRejected ApprovalDecision = "REJECTED"

	// ApprovalEdit indicates the action was approved with state modifications
	ApprovalEdit ApprovalDecision = "EDIT"

	// ApprovalSkip indicates approval was skipped (e.g., timeout, auto-bypass)
	ApprovalSkip ApprovalDecision = "SKIP"
)

// ApprovalGuard is a function that evaluates whether approval is needed for a node.
// It receives the current state and returns:
//   - needsApproval: true if human approval is required
//   - reason: explanation of why approval is needed (shown to reviewer)
//   - error: if evaluation fails
//
// Example:
//
//	guard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
//	    email := state.GetFromView(view, emailKey)
//	    if containsSensitiveInfo(email) {
//	        return true, "Email contains confidential information", nil
//	    }
//	    return false, "", nil  // No approval needed
//	}
type ApprovalGuard func(ctx context.Context, view state.ReadView) (needsApproval bool, reason string, err error)

// ApprovalResponse contains the human reviewer's decision and metadata.
// This is provided when resuming execution after an approval interrupt.
type ApprovalResponse struct {
	// Decision is the approval outcome (APPROVED, REJECTED, EDIT, SKIP)
	Decision ApprovalDecision `json:"decision"`

	// Reason is the human's explanation for their decision
	Reason string `json:"reason"`

	// User identifies who made the approval decision
	User string `json:"user"`

	// Timestamp when the decision was made
	Timestamp time.Time `json:"timestamp"`

	// Edits contains state modifications if Decision is ApprovalEdit
	// These updates are applied to state before resuming execution
	Edits state.Updates `json:"edits,omitempty"`

	// Annotations contains custom metadata about the approval
	// (e.g., department, policy version, risk score)
	Annotations map[string]any `json:"annotations,omitempty"`
}

// ApprovalInfo describes why approval is needed for a specific node.
// This information is available in the error when an approval interrupt occurs.
type ApprovalInfo struct {
	// NodeName is the node requiring approval
	NodeName string

	// Reason explains why approval is needed (from ApprovalGuard)
	Reason string

	// RequestedAt is when approval was first requested
	RequestedAt time.Time

	// TimeoutAt is when the approval request expires (nil if no timeout)
	TimeoutAt *time.Time

	// State is a snapshot of relevant state for review
	State map[string]any
}

// ApprovalConfig holds approval workflow configuration for a node.
type ApprovalConfig struct {
	// Guard evaluates whether approval is needed
	Guard ApprovalGuard

	// FeedbackAnnotation enables recording approval decisions in message history
	FeedbackAnnotation bool

	// Timeout specifies how long to wait for approval before auto-rejecting
	// Zero duration means no timeout
	Timeout time.Duration

	// StateSnapshot lists state keys to include in approval request
	// Empty means include all state
	StateSnapshot []string
}

// ApprovalOption configures approval behavior for a node interrupt.
type ApprovalOption func(*ApprovalConfig)

// WithApprovalGuard sets a conditional guard that determines if approval is needed.
// The guard is evaluated before interrupting - if it returns false, execution continues
// automatically without requiring approval.
//
// Example:
//
//	graph.WithApprovalGuard(func(ctx context.Context, view state.ReadView) (bool, string, error) {
//	    if containsSensitiveData(view) {
//	        return true, "Contains sensitive data", nil
//	    }
//	    return false, "", nil  // Auto-continue
//	})
func WithApprovalGuard(guard ApprovalGuard) ApprovalOption {
	return func(c *ApprovalConfig) {
		c.Guard = guard
	}
}

// WithFeedbackAnnotation enables automatic recording of approval decisions in message history.
// When enabled, a system message is appended with the approval decision, user, and reason.
//
// Example annotation:
//
//	SystemMessage{
//	    Content: "Human approval: APPROVED - Reviewed and approved (by alice@example.com)",
//	    Metadata: {"approval_node": "send_email", "decision": "APPROVED", "user": "alice@example.com"},
//	}
func WithFeedbackAnnotation(enabled bool) ApprovalOption {
	return func(c *ApprovalConfig) {
		c.FeedbackAnnotation = enabled
	}
}

// WithApprovalTimeout sets a maximum wait time for approval.
// If approval is not provided within the timeout, the request can be auto-rejected
// or handled according to a timeout policy.
//
// Example:
//
//	graph.WithApprovalTimeout(24 * time.Hour)  // 24-hour SLA for approval
func WithApprovalTimeout(timeout time.Duration) ApprovalOption {
	return func(c *ApprovalConfig) {
		c.Timeout = timeout
	}
}

// WithStateSnapshot specifies which state keys to include in the approval request.
// This allows reviewers to see relevant context without exposing the entire state.
//
// Example:
//
//	graph.WithStateSnapshot("email_draft", "recipient", "subject")
func WithStateSnapshot(keys ...string) ApprovalOption {
	return func(c *ApprovalConfig) {
		c.StateSnapshot = keys
	}
}
