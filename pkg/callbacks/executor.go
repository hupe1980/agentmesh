package callbacks

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// safeExecuteBeforeModel wraps a BeforeModelCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteBeforeModel(ctx context.Context, cb BeforeModelCallback, s graph.StateWriter) (result message.Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("BeforeModelCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, s)
}

// safeExecuteAfterModel wraps an AfterModelCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteAfterModel(ctx context.Context, cb AfterModelCallback, s graph.StateWriter, response message.Message) (result message.Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("AfterModelCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, s, response)
}

// safeExecuteOnModelError wraps an OnModelErrorCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteOnModelError(ctx context.Context, cb OnModelErrorCallback, s graph.StateWriter, modelErr error) (result message.Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("OnModelErrorCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, s, modelErr)
}

// safeExecuteBeforeTool wraps a BeforeToolCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteBeforeTool(ctx context.Context, cb BeforeToolCallback, s graph.StateWriter, call message.ToolCall) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("BeforeToolCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, s, call)
}

// safeExecuteAfterTool wraps an AfterToolCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteAfterTool(ctx context.Context, cb AfterToolCallback, s graph.StateWriter, call message.ToolCall, toolResult any) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("AfterToolCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, s, call, toolResult)
}

// safeExecuteOnToolError wraps an OnToolErrorCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteOnToolError(ctx context.Context, cb OnToolErrorCallback, s graph.StateWriter, call message.ToolCall, toolErr error) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("OnToolErrorCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, s, call, toolErr)
}
