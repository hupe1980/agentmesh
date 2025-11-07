package callbacks

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// safeExecuteBeforeModel wraps a BeforeModelCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteBeforeModel(ctx context.Context, cb BeforeModelCallback, messages []message.Message) (result message.Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("BeforeModelCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, messages)
}

// safeExecuteAfterModel wraps an AfterModelCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteAfterModel(ctx context.Context, cb AfterModelCallback, messages []message.Message, response message.Message) (result message.Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("AfterModelCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, messages, response)
}

// safeExecuteBeforeTool wraps a BeforeToolCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteBeforeTool(ctx context.Context, cb BeforeToolCallback, call message.ToolCall) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("BeforeToolCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, call)
}

// safeExecuteAfterTool wraps an AfterToolCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteAfterTool(ctx context.Context, cb AfterToolCallback, call message.ToolCall, toolResult any) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("AfterToolCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, call, toolResult)
}

// safeExecuteOnToolError wraps an OnToolErrorCallback with panic recovery.
// If the callback panics, it returns an error describing the panic.
func safeExecuteOnToolError(ctx context.Context, cb OnToolErrorCallback, call message.ToolCall, toolErr error) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("OnToolErrorCallback panicked: %v", r)
			result = nil
		}
	}()

	return cb(ctx, call, toolErr)
}
