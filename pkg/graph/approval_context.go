package graph

import (
	"context"
	"errors"
)

// ErrApprovalRequired indicates that a node requires human approval before continuing.
// This error wraps ApprovalInfo which contains details about why approval is needed.
var ErrApprovalRequired = errors.New("graph: approval required")

// approvalRequiredError is the concrete error type that wraps ApprovalInfo.
type approvalRequiredError struct {
	info *ApprovalInfo
}

// Error implements the error interface.
func (e *approvalRequiredError) Error() string {
	return "graph: approval required for node " + e.info.NodeName + ": " + e.info.Reason
}

// Unwrap allows errors.Is and errors.As to work with ErrApprovalRequired.
func (e *approvalRequiredError) Unwrap() error {
	return ErrApprovalRequired
}

// NewApprovalRequiredError creates an error indicating approval is required.
func NewApprovalRequiredError(info *ApprovalInfo) error {
	return &approvalRequiredError{info: info}
}

// IsApprovalRequired checks if an error indicates approval is required.
//
// Example:
//
//	for output, err := range compiled.Run(ctx, input) {
//	    if graph.IsApprovalRequired(err) {
//	        // Handle approval workflow
//	        info := graph.ApprovalInfoFromError(err)
//	        fmt.Printf("Approval needed: %s\n", info.Reason)
//	        break
//	    }
//	}
func IsApprovalRequired(err error) bool {
	return errors.Is(err, ErrApprovalRequired)
}

// ApprovalInfoFromError extracts ApprovalInfo from an approval required error.
// Returns nil if the error is not an approval required error.
//
// Example:
//
//	if IsApprovalRequired(err) {
//	    info := ApprovalInfoFromError(err)
//	    fmt.Printf("Node: %s, Reason: %s\n", info.NodeName, info.Reason)
//	}
func ApprovalInfoFromError(err error) *ApprovalInfo {
	var approvalErr *approvalRequiredError
	if errors.As(err, &approvalErr) {
		return approvalErr.info
	}
	return nil
}

// approvalContextKey is the context key for approval responses.
type approvalContextKey struct{}

// WithApprovalResponse adds an approval response to the context.
// This is used internally when resuming execution with an approval decision.
func WithApprovalResponse(ctx context.Context, nodeName string, response *ApprovalResponse) context.Context {
	approvals := getApprovalMap(ctx)
	newApprovals := make(map[string]*ApprovalResponse)
	for k, v := range approvals {
		newApprovals[k] = v
	}
	newApprovals[nodeName] = response
	return context.WithValue(ctx, approvalContextKey{}, newApprovals)
}

// ApprovalFromContext retrieves the approval response for a specific node from context.
// Returns nil if no approval is present for the node.
//
// Example (in node implementation):
//
//	func (n *SendEmailNode) Execute(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	    approval := graph.ApprovalFromContext(ctx, "send_email")
//	    if approval != nil {
//	        switch approval.Decision {
//	        case graph.ApprovalRejected:
//	            return []string{graph.EndNode}, state.Updates{"sent": false}, nil
//	        case graph.ApprovalApproved:
//	            // Proceed with sending
//	        }
//	    }
//	    // ... node logic
//	}
func ApprovalFromContext(ctx context.Context, nodeName string) *ApprovalResponse {
	approvals := getApprovalMap(ctx)
	return approvals[nodeName]
}

// getApprovalMap retrieves the approval map from context.
func getApprovalMap(ctx context.Context) map[string]*ApprovalResponse {
	approvals, ok := ctx.Value(approvalContextKey{}).(map[string]*ApprovalResponse)
	if !ok {
		return make(map[string]*ApprovalResponse)
	}
	return approvals
}

// CheckApproval is a helper function that nodes can use to check for approval.
// It returns the approval response if present, or an error if approval is required
// but not provided.
//
// Example:
//
//	func (n *CriticalNode) Execute(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	    approval, err := graph.CheckApproval(ctx, "critical_node")
//	    if err != nil {
//	        return nil, nil, err  // Approval required but not provided
//	    }
//	    if approval.Decision == graph.ApprovalRejected {
//	        return []string{graph.EndNode}, state.Updates{"status": "rejected"}, nil
//	    }
//	    // Proceed with critical operation
//	}
func CheckApproval(ctx context.Context, nodeName string, required bool) (*ApprovalResponse, error) {
	approval := ApprovalFromContext(ctx, nodeName)
	if approval == nil && required {
		return nil, errors.New("graph: approval required for node " + nodeName + " but not provided")
	}
	return approval, nil
}
