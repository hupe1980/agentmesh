package state

import (
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// ExecutionResult represents a message event from node execution.
// This wraps a message with execution metadata (node name, timestamp, updates, errors).
//
// Each ExecutionResult contains exactly ONE Message (the embedded message.Message field).
// During streaming (graph.Run iteration), results are emitted one at a time as nodes produce messages.
//
// To get all accumulated messages:
//   - Use Collect() to gather all results from the iterator
//   - Or access Reader.MessagesSnapshot() after execution
//
// ERROR RESULTS:
//   - Message is nil, only Err is set
type ExecutionResult struct {
	// Single message content (one message per result)
	Message message.Message

	// Execution metadata
	ID        string    // UUID result identifier (generated automatically)
	GraphID   string    // Graph run ID (hierarchical for subgraphs: "parent:child")
	Node      string    // Node that created this result
	Timestamp time.Time // Creation timestamp

	// Node execution results
	Updates map[string]any // State updates from the node
	Err     error          // Error if node execution failed
	Partial bool           // True if this is an intermediate streaming result (not applied to state)
}

// NewExecutionResult creates an execution result wrapping a message with metadata.
// Automatically generates a UUID for the result ID.
// Used internally for state management message wrapping.
func NewExecutionResult(msg message.Message, graphID, node string) *ExecutionResult {
	return &ExecutionResult{
		Message:   msg,
		ID:        uuid.New().String(),
		GraphID:   graphID,
		Node:      node,
		Timestamp: time.Now(),
	}
}

// String returns a human-readable representation of the execution result.
func (e *ExecutionResult) String() string {
	if e.Message != nil {
		return fmt.Sprintf("[%s:%s] %s", e.GraphID, e.Node, e.Message.Type())
	}
	return fmt.Sprintf("[%s:%s]", e.GraphID, e.Node)
}

// Clone creates a deep copy of the execution result and wrapped message.
func (e *ExecutionResult) Clone() *ExecutionResult {
	clone := &ExecutionResult{
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

// ExtractMessages extracts the underlying messages from ExecutionResults.
// Helper for accessing message content when ExecutionResult metadata is not needed.
func ExtractMessages(results []ExecutionResult) []message.Message {
	if len(results) == 0 {
		return nil
	}
	messages := make([]message.Message, 0, len(results))
	for i := range results {
		if results[i].Message != nil {
			messages = append(messages, results[i].Message)
		}
	}
	return messages
}
