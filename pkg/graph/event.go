package graph

import (
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// Event represents a message event from node execution.
//
// Each Event contains exactly ONE Message (the embedded message.Message field).
// During streaming (graph.Run iteration), events are emitted one at a time as nodes produce messages.
//
// To get all accumulated messages:
//   - Use Collect() to gather all events from the iterator
//   - Or access graph.State().EventsSnapshot() after execution
//
// ERROR EVENTS:
//   - Message is nil, only Err is set
type Event struct {
	// Single message content (one message per event)
	Message message.Message

	// Execution metadata
	ID        string    // UUID event identifier (generated automatically)
	GraphID   string    // Graph run ID (hierarchical for subgraphs: "parent:child")
	Node      string    // Node that created this event
	Timestamp time.Time // Creation timestamp

	// Node execution results
	Updates map[string]any // State updates from the node
	Err     error          // Error if node execution failed
}

// NewEvent creates an event wrapping a message with metadata.
// Automatically generates a UUID for the event ID.
// Used internally for state management message wrapping.
func NewEvent(msg message.Message, graphID, node string) *Event {
	return &Event{
		Message:   msg,
		ID:        uuid.New().String(),
		GraphID:   graphID,
		Node:      node,
		Timestamp: time.Now(),
	}
}

// String returns a human-readable representation of the event.
func (e *Event) String() string {
	if e.Message != nil {
		return fmt.Sprintf("[%s:%s] %s", e.GraphID, e.Node, e.Message.Type())
	}
	return fmt.Sprintf("[%s:%s]", e.GraphID, e.Node)
}

// Clone creates a deep copy of the event and wrapped message.
func (e *Event) Clone() *Event {
	clone := &Event{
		ID:        e.ID,
		GraphID:   e.GraphID,
		Node:      e.Node,
		Timestamp: e.Timestamp,
		Err:       e.Err,
	}
	if e.Message != nil {
		clone.Message = e.Message.Clone()
	}
	if e.Updates != nil {
		clone.Updates = make(map[string]any, len(e.Updates))
		maps.Copy(clone.Updates, e.Updates)
	}
	return clone
}

// ExtractMessages extracts the underlying messages from Events.
// Helper for accessing message content when Event metadata is not needed.
func ExtractMessages(events []Event) []message.Message {
	if len(events) == 0 {
		return nil
	}
	messages := make([]message.Message, 0, len(events))
	for i := range events {
		if events[i].Message != nil {
			messages = append(messages, events[i].Message)
		}
	}
	return messages
}
