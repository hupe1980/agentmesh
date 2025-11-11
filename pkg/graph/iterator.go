package graph

import "iter"

// Last consumes an iterator and returns only the final event and error.
// This is useful for getting the result of a graph run without processing intermediate steps.
//
// Note: Each Event contains a single Message from node execution.
// To get ALL accumulated messages, use Collect to gather all events, or access
// the graph's State().EventsSnapshot() directly after consuming the iterator.
func Last(seq iter.Seq2[Event, error]) (Event, error) {
	var lastEvent Event
	var lastErr error

	for event, err := range seq {
		lastEvent = event
		if err != nil {
			lastErr = err
			break
		}
	}

	return lastEvent, lastErr
}

// Collect gathers all events from an iterator into a slice.
// The final error (if any) is returned separately. This is useful for testing
// and debugging to inspect the full execution trace.
func Collect(seq iter.Seq2[Event, error]) ([]Event, error) {
	events := make([]Event, 0)
	var lastErr error

	for event, err := range seq {
		if err != nil {
			lastErr = err
			break
		}
		events = append(events, event)
	}

	return events, lastErr
}
