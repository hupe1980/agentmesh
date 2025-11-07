package graph

// Error Wrapping Standards
//
// AgentMesh follows consistent error wrapping patterns for better error diagnosis
// in production environments.
//
// # Error Wrapping Guidelines
//
// 1. **Always use %w for wrapping errors:**
//
//	return fmt.Errorf("operation failed: %w", err)
//
// 2. **Include context in error messages:**
//
//	return fmt.Errorf("node %q execution failed: %w", nodeName, err)
//	return fmt.Errorf("graph validation: missing START node: %w", ErrMissingStart)
//
// 3. **Use package/component prefixes for clarity:**
//
//	return fmt.Errorf("react agent: failed to list tools: %w", err)
//	return fmt.Errorf("state manager: no checkpointer configured for run %q", runID)
//
// 4. **Sentinel errors should be wrapped with %w:**
//
//	if ctx == nil {
//	    return fmt.Errorf("graph execution: %w", ErrNilContext)
//	}
//
// 5. **Avoid errors.New for error propagation:**
//
//	// Bad: loses error chain
//	return errors.New("something failed")
//
//	// Good: preserves error chain
//	return fmt.Errorf("something failed: %w", err)
//
// 6. **Use errors.Join for multiple errors:**
//
//	if err1 != nil && err2 != nil {
//	    return errors.Join(err1, err2)
//	}
//
// # Structured Errors
//
// Use custom error types for errors that need special handling:
//
//	type NodeExecutionError struct {
//	    Node      string
//	    Superstep int64
//	    Cause     error
//	}
//
//	func (e *NodeExecutionError) Error() string {
//	    return fmt.Sprintf("node %q failed at superstep %d: %v", e.Node, e.Superstep, e.Cause)
//	}
//
//	func (e *NodeExecutionError) Unwrap() error {
//	    return e.Cause
//	}
//
// This allows consumers to use errors.Is() and errors.As() for error inspection.
//
// # Error Message Format
//
// Error messages should:
//   - Start with lowercase (unless referring to a proper noun)
//   - Be descriptive and actionable
//   - Include relevant context (node names, IDs, values)
//   - Not end with punctuation
//
// Examples:
//
//	"node %q: execution timeout after %v"
//	"graph validation: edge from %q to %q references unknown node"
//	"checkpoint: failed to load run %q at superstep %d: %w"
