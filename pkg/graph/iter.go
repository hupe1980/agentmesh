package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
)

// ErrEmptySequence is returned when trying to get the last element of an empty sequence.
var ErrEmptySequence = errors.New("graph: empty sequence")

// Last returns the last value from an iterator sequence.
// Returns an error if the sequence produces an error or is empty.
//
// ERROR HANDLING:
//   - Returns immediately when iterator yields any error (err != nil)
//   - Use this for blocking/non-streaming execution
//
// Example:
//
//	lastMsg, err := graph.Last(g.Run(ctx, input))
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

// Collect gathers all values from an iterator sequence into a slice.
// Returns an error if the sequence produces an error.
//
// Example:
//
//	results, err := graph.Collect(g.Run(ctx, input))
//	if err != nil {
//	    return fmt.Errorf("execution failed: %w", err)
//	}
func Collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	results := make([]T, 0)
	for val, err := range seq {
		if err != nil {
			return results, err
		}
		results = append(results, val)
	}
	return results, nil
}

// LastStructured extracts the last value from an iterator and unmarshals
// content into the specified type T. The output type O must implement
// [fmt.Stringer]. This is useful for extracting structured output from
// agent execution.
//
// Example with agent:
//
//	type MovieReview struct {
//	    Title   string `json:"title"`
//	    Rating  int    `json:"rating"`
//	    Summary string `json:"summary"`
//	}
//
//	agent, _ := agent.NewReAct(model, agent.WithOutputSchema(outputSchema))
//	review, err := graph.LastStructured[MovieReview](agent.Run(ctx, messages))
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Rating: %d/5\n", review.Rating)
func LastStructured[T any, O fmt.Stringer](seq iter.Seq2[O, error]) (*T, error) {
	last, err := Last(seq)
	if err != nil {
		return nil, err
	}

	content := last.String()

	var result T
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("graph: failed to parse structured output: %w", err)
	}

	return &result, nil
}
