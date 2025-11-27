package event

import "time"

// Event represents a unified event that can be published by any component.
// This structure is used across graph, model, tool, and node execution.
type Event struct {
	// Core identification
	Type      Type      `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	RunID     string    `json:"run_id,omitempty"`

	// Execution context
	Superstep int    `json:"superstep,omitempty"`
	Node      string `json:"node,omitempty"` // Node name (for graph events)

	// Payload (event-specific data)
	Data map[string]any `json:"data,omitempty"`

	// Error information
	Error string `json:"error,omitempty"`

	// Duration (for complete events)
	Duration time.Duration `json:"duration,omitempty"`
}

// Type represents the type of execution event.
type Type string

// Graph lifecycle events
const (
	EventGraphStart    Type = "graph.start"
	EventGraphComplete Type = "graph.complete"
	EventGraphError    Type = "graph.error"
)

// Node lifecycle events
const (
	EventNodeQueued   Type = "node.queued"
	EventNodeStart    Type = "node.start"
	EventNodeComplete Type = "node.complete"
	EventNodeError    Type = "node.error"
)

// Model lifecycle events
const (
	EventModelStart    Type = "model.start"
	EventModelComplete Type = "model.complete"
	EventModelError    Type = "model.error"
	EventModelStream   Type = "model.stream"
)

// Tool lifecycle events
const (
	EventToolStart    Type = "tool.start"
	EventToolComplete Type = "tool.complete"
	EventToolError    Type = "tool.error"
)

// State and execution control events
const (
	EventStateUpdate       Type = "state.update"
	EventSuperstepStart    Type = "superstep.start"
	EventSuperstepComplete Type = "superstep.complete"
	EventCheckpointSave    Type = "checkpoint.save"
	EventCheckpointLoad    Type = "checkpoint.load"
	EventCheckpointError   Type = "checkpoint.error"
	EventInterrupt         Type = "interrupt"
	EventResume            Type = "resume"
)
