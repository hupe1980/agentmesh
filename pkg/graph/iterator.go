package graph

import (
	"iter"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Last returns the last element from an iterator sequence.
// Returns an error if the sequence produces an error or is empty.
//
// Example:
//
//	result, err := graph.Last(runnable.Run(ctx, messages))
func Last(seq iter.Seq2[state.ExecutionResult, error]) (state.ExecutionResult, error) {
	var last state.ExecutionResult
	var lastErr error
	hasValue := false

	// Must consume entire iterator to avoid breaking range-over-func protocol
	for val, err := range seq {
		if err != nil {
			// Store error but keep consuming
			lastErr = err
		} else {
			// Only update last value if no error
			last = val
			hasValue = true
		}
	}

	if lastErr != nil {
		return state.ExecutionResult{}, lastErr
	}

	if !hasValue {
		return state.ExecutionResult{}, ErrEmptySequence
	}

	return last, lastErr
}

// Collect collects all execution results from an iterator sequence.
// Returns an error if the sequence produces an error.
//
// Example:
//
//	results, err := graph.Collect(runnable.Run(ctx, messages))
func Collect(seq iter.Seq2[state.ExecutionResult, error]) ([]state.ExecutionResult, error) {
	results := make([]state.ExecutionResult, 0)
	for result, err := range seq {
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// CollectMessages collects all messages from execution results in an iterator sequence.
// Returns an error if the sequence produces an error.
//
// Example:
//
//	messages, err := graph.CollectMessages(runnable.Run(ctx, messages))
func CollectMessages(seq iter.Seq2[state.ExecutionResult, error]) ([]state.ExecutionResult, error) {
	var messages []state.ExecutionResult
	for result, err := range seq {
		if err != nil {
			return messages, err
		}
		if result.Message != nil {
			messages = append(messages, result)
		}
	}
	return messages, nil
}

// ErrEmptySequence is returned when trying to get the last element of an empty sequence.
var ErrEmptySequence = &IteratorError{msg: "empty sequence"}

// IteratorError represents an iterator operation error.
type IteratorError struct {
	msg string
}

func (e *IteratorError) Error() string {
	return e.msg
}
