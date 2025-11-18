package graph

import (
	"iter"
)

// Last returns the last value from an iterator sequence.
// Returns an error if the sequence produces an error or is empty.
//
// ERROR HANDLING:
//   - Returns immediately when iterator yields any error (err != nil)
//   - Use this for blocking/non-streaming execution
//
// Example:
//
//	lastMsg, err := graph.Last(runnable.Run(ctx, input))
//	if err != nil {
//	    return fmt.Errorf("execution failed: %w", err)
//	}
func Last[T any](seq iter.Seq2[T, error]) (T, error) {
	var last T
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
		var zero T
		return zero, lastErr
	}

	if !hasValue {
		var zero T
		return zero, ErrEmptySequence
	}

	return last, lastErr
}

// Collect collects all values from an iterator sequence.
// Returns an error if the sequence produces an error.
//
// Example:
//
//	results, err := graph.Collect(runnable.Run(ctx, input))
//	if err != nil {
//	    return fmt.Errorf("execution failed: %w", err)
//	}
func Collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	results := make([]T, 0)
	for val, err := range seq {
		if err != nil {
			return results, err
		}
		// Note: We include zero values in the slice
		results = append(results, val)
	}
	return results, nil
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
