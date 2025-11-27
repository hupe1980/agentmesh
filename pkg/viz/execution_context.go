package viz

import "context"

// contextKey is a private type for context keys to avoid collisions
type executionContextKey string

const (
	// executionControllerKey is the context key for the execution controller
	executionControllerKey executionContextKey = "executionController"
)

// WithExecutionController attaches an ExecutionController to the context.
// This allows the graph execution to check for breakpoints and pause conditions.
func WithExecutionController(ctx context.Context, controller *ExecutionController) context.Context {
	return context.WithValue(ctx, executionControllerKey, controller)
}

// ExecutionControllerFromContext retrieves the ExecutionController from the context.
// Returns nil if no controller is attached (normal execution without debugging).
func ExecutionControllerFromContext(ctx context.Context) *ExecutionController {
	if controller, ok := ctx.Value(executionControllerKey).(*ExecutionController); ok {
		return controller
	}
	return nil
}
