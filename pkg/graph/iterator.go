package graph

import (
	"iter"

	"github.com/hupe1980/agentmesh/pkg/message"
)

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

// CollectMessages directly collects all messages from an iterator.
// This is a convenience function that extracts messages from events as they arrive,
// avoiding the need to collect events first and then extract messages separately.
// The final error (if any) is returned separately.
func CollectMessages(seq iter.Seq2[Event, error]) ([]message.Message, error) {
	messages := make([]message.Message, 0)
	var lastErr error

	for event, err := range seq {
		if err != nil {
			lastErr = err
			break
		}
		if event.Message != nil {
			messages = append(messages, event.Message)
		}
	}

	return messages, lastErr
}

// LastMessage consumes an iterator and returns only the final message and error.
// This is useful for getting the final result of a graph run without processing
// intermediate messages.
func LastMessage(seq iter.Seq2[Event, error]) (message.Message, error) {
	event, err := Last(seq)
	if err != nil {
		return nil, err
	}
	return event.Message, nil
}
