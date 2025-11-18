package graph

import (
	"iter"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// Last returns the last message from an iterator sequence.
// Returns an error if the sequence produces an error or is empty.
//
// ERROR HANDLING:
//   - Returns immediately when iterator yields any error (err != nil)
//   - Use this for blocking/non-streaming execution
//
// Example:
//
//	lastMsg, err := graph.Last(runnable.Run(ctx, messages))
//	if err != nil {
//	    return fmt.Errorf("execution failed: %w", err)
//	}
func Last(seq iter.Seq2[message.Message, error]) (message.Message, error) {
	var last message.Message
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
		return nil, lastErr
	}

	if !hasValue {
		return nil, ErrEmptySequence
	}

	return last, lastErr
}

// Collect collects all messages from an iterator sequence.
// Returns an error if the sequence produces an error.
//
// Example:
//
//	messages, err := graph.Collect(runnable.Run(ctx, messages))
//	if err != nil {
//	    return fmt.Errorf("execution failed: %w", err)
//	}
func Collect(seq iter.Seq2[message.Message, error]) ([]message.Message, error) {
	messages := make([]message.Message, 0)
	for msg, err := range seq {
		if err != nil {
			return messages, err
		}
		if msg != nil {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

// CollectMessages collects all messages from an iterator sequence.
// This is an alias for Collect for backward compatibility.
//
// Example:
//
//	messages, err := graph.CollectMessages(runnable.Run(ctx, messages))
func CollectMessages(seq iter.Seq2[message.Message, error]) ([]message.Message, error) {
	return Collect(seq)
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
