package graph

import (
	"iter"

	"github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// Last consumes an iterator and returns only the final execution result and error.
// This is useful for getting the result of a graph run without processing intermediate steps.
//
// Note: Each state.ExecutionResult contains a single Message from node execution.
// To get ALL accumulated messages, use Collect to gather all results, or access
// the graph's State().MessagesSnapshot() directly after consuming the iterator.
func Last(seq iter.Seq2[state.ExecutionResult, error]) (state.ExecutionResult, error) {
	var lastResult state.ExecutionResult
	var lastErr error

	for result, err := range seq {
		lastResult = result
		if err != nil {
			lastErr = err
			break
		}
	}

	return lastResult, lastErr
}

// Collect gathers all execution results from an iterator into a slice.
// The final error (if any) is returned separately. This is useful for testing
// and debugging to inspect the full execution trace.
func Collect(seq iter.Seq2[state.ExecutionResult, error]) ([]state.ExecutionResult, error) {
	results := make([]state.ExecutionResult, 0)
	var lastErr error

	for result, err := range seq {
		if err != nil {
			lastErr = err
			break
		}
		results = append(results, result)
	}

	return results, lastErr
}

// CollectGeneric gathers all values from a generic iterator into a slice.
// This works with any Runnable[I, O] type, not just state.ExecutionResult.
// The final error (if any) is returned separately.
func CollectGeneric[T any](seq iter.Seq2[T, error]) ([]T, error) {
	results := make([]T, 0)
	var lastErr error

	for result, err := range seq {
		if err != nil {
			lastErr = err
			break
		}
		results = append(results, result)
	}

	return results, lastErr
}

// CollectMessages directly collects all messages from an iterator.
// This is a convenience function that extracts messages from execution results as they arrive,
// avoiding the need to collect results first and then extract messages separately.
// The final error (if any) is returned separately.
func CollectMessages(seq iter.Seq2[state.ExecutionResult, error]) ([]message.Message, error) {
	messages := make([]message.Message, 0)
	var lastErr error

	for result, err := range seq {
		if err != nil {
			lastErr = err
			break
		}
		if result.Message != nil {
			messages = append(messages, result.Message)
		}
	}

	return messages, lastErr
}

// LastMessage consumes an iterator and returns only the final message and error.
// This is useful for getting the final result of a graph run without processing
// intermediate messages.
func LastMessage(seq iter.Seq2[state.ExecutionResult, error]) (message.Message, error) {
	result, err := Last(seq)
	if err != nil {
		return nil, err
	}
	return result.Message, nil
}
