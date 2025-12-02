package viz

import (
	"time"

	"github.com/hupe1980/agentmesh/pkg/event"
)

// ExecutionEvent represents a comprehensive execution event with rich metadata.
// This is the enhanced event structure for the redesigned visualization system.
type ExecutionEvent struct {
	ID        string    `json:"id"`                  // Unique event ID
	RunID     string    `json:"run_id"`              // Run identifier
	Type      EventType `json:"type"`                // Event type
	Timestamp time.Time `json:"timestamp"`           // Event timestamp
	Superstep int       `json:"superstep,omitempty"` // Superstep number
	Node      string    `json:"node,omitempty"`      // Node name

	// Detailed payload with context-specific data
	Payload EventPayload `json:"payload"`

	// Execution metrics
	Duration time.Duration `json:"duration,omitempty"` // Execution duration
	Memory   uint64        `json:"memory,omitempty"`   // Memory usage in bytes

	// Execution context
	Thread string `json:"thread,omitempty"` // Thread ID for parallel execution
	Parent string `json:"parent,omitempty"` // Parent event ID for nested events

	// Visualization data
	NodeStatus            NodeStatus     `json:"node_status,omitempty"`    // Visual state of the node
	IncomingEdges         []string       `json:"incoming_edges,omitempty"` // Nodes that triggered this
	OutgoingEdges         []string       `json:"outgoing_edges,omitempty"` // Next nodes to execute
	VisualizationMetadata map[string]any `json:"viz_metadata,omitempty"`   // Extra visualization hints
}

// EventPayload contains context-specific event data.
// Note: StateDiff is defined in diff.go
type EventPayload struct {
	// State information (for state update events)
	StateBefore map[string]any `json:"state_before,omitempty"`
	StateAfter  map[string]any `json:"state_after,omitempty"`
	StateDiff   []StateDiff    `json:"state_diff,omitempty"` // Using StateDiff from diff.go

	// Model-specific data
	ModelName    string         `json:"model_name,omitempty"`
	ModelRequest map[string]any `json:"model_request,omitempty"`
	ModelReply   map[string]any `json:"model_reply,omitempty"`

	// Tool-specific data
	ToolName   string         `json:"tool_name,omitempty"`
	ToolArgs   map[string]any `json:"tool_args,omitempty"`
	ToolResult any            `json:"tool_result,omitempty"`

	// Token usage and costs
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	TotalTokens  int     `json:"total_tokens,omitempty"`
	EstCostUSD   float64 `json:"est_cost_usd,omitempty"`

	// Error details
	Error      string `json:"error,omitempty"`
	ErrorStack string `json:"error_stack,omitempty"`

	// Custom metadata
	Metadata map[string]any `json:"metadata,omitempty"`
}

// EventFilter defines criteria for querying events.
type EventFilter struct {
	Types      []EventType `json:"types,omitempty"`       // Filter by event types
	Nodes      []string    `json:"nodes,omitempty"`       // Filter by node names
	StartTime  *time.Time  `json:"start_time,omitempty"`  // Filter by start time
	EndTime    *time.Time  `json:"end_time,omitempty"`    // Filter by end time
	SearchText string      `json:"search_text,omitempty"` // Full-text search
	Limit      int         `json:"limit,omitempty"`       // Maximum results
	Offset     int         `json:"offset,omitempty"`      // Pagination offset
}

// ConvertGraphEvent converts a event.Event to an ExecutionEvent.
func ConvertGraphEvent(graphEvent event.Event, runID string) ExecutionEvent {
	event := ExecutionEvent{
		ID:        generateEventID(),
		RunID:     runID,
		Type:      mapGraphEventType(graphEvent.Type),
		Timestamp: graphEvent.Timestamp,
		Superstep: graphEvent.Superstep,
		Node:      graphEvent.Node,
		Duration:  graphEvent.Duration,
		Payload: EventPayload{
			Error:    graphEvent.Error,
			Metadata: graphEvent.Data,
		},
	}

	// Extract additional data from graph event
	if graphEvent.Data != nil {
		extractPayloadData(&event.Payload, graphEvent.Data)
	}

	return event
}

// mapGraphEventType maps event.Type to viz.EventType.
//
//nolint:gocyclo // Simple event type mapping switch
func mapGraphEventType(graphType event.Type) EventType {
	switch graphType {
	case event.EventGraphStart:
		return EventGraphStart
	case event.EventGraphComplete:
		return EventGraphComplete
	case event.EventGraphError:
		return EventGraphError
	case event.EventSuperstepStart:
		return EventStepStart
	case event.EventSuperstepComplete:
		return EventStepEnd
	case event.EventNodeQueued:
		return EventNodeQueued
	case event.EventNodeStart:
		return EventNodeStart
	case event.EventNodeComplete:
		return EventNodeComplete
	case event.EventNodeError:
		return EventNodeError
	case event.EventStateUpdate:
		return EventStateUpdate
	case event.EventCheckpointSave:
		return EventCheckpoint
	case event.EventCheckpointLoad:
		return EventCheckpointLoad
	case event.EventCheckpointError:
		return EventCheckpointError
	case event.EventInterrupt:
		return EventInterrupt
	case event.EventResume:
		return EventResume
	case event.EventModelStart:
		return EventModelStart
	case event.EventModelComplete:
		return EventModelComplete
	case event.EventModelError:
		return EventModelError
	case event.EventToolStart:
		return EventToolStart
	case event.EventToolComplete:
		return EventToolComplete
	case event.EventToolError:
		return EventToolError
	default:
		return EventType(string(graphType))
	}
}

// extractPayloadData extracts structured data from event metadata.
//
//nolint:gocyclo // Straightforward data extraction from map
func extractPayloadData(payload *EventPayload, data map[string]any) {
	// Extract model information (support both "model" and "model_name")
	if modelName, ok := data["model_name"].(string); ok {
		payload.ModelName = modelName
	} else if model, ok := data["model"].(string); ok {
		payload.ModelName = model
	}
	if modelRequest, ok := data["model_request"].(map[string]any); ok {
		payload.ModelRequest = modelRequest
	}
	if modelReply, ok := data["model_reply"].(map[string]any); ok {
		payload.ModelReply = modelReply
	}

	// Extract tool information
	if toolName, ok := data["tool_name"].(string); ok {
		payload.ToolName = toolName
	}
	if toolArgs, ok := data["tool_args"].(map[string]any); ok {
		payload.ToolArgs = toolArgs
	}
	if toolResult, ok := data["tool_result"]; ok {
		payload.ToolResult = toolResult
	}

	// Extract token usage (support both top-level and nested in "usage" map)
	if inputTokens, ok := data["input_tokens"].(int); ok {
		payload.InputTokens = inputTokens
	}
	if outputTokens, ok := data["output_tokens"].(int); ok {
		payload.OutputTokens = outputTokens
	}
	if totalTokens, ok := data["total_tokens"].(int); ok {
		payload.TotalTokens = totalTokens
	}
	if cost, ok := data["cost_usd"].(float64); ok {
		payload.EstCostUSD = cost
	}

	// Also check for nested usage information (from model events)
	if usage, ok := data["usage"].(map[string]any); ok {
		if promptTokens, ok := usage["prompt_tokens"].(int); ok {
			payload.InputTokens = promptTokens
		}
		if completionTokens, ok := usage["completion_tokens"].(int); ok {
			payload.OutputTokens = completionTokens
		}
		if totalTokens, ok := usage["total_tokens"].(int); ok {
			payload.TotalTokens = totalTokens
		}
	}

	// Extract state information
	if stateBefore, ok := data["state_before"].(map[string]any); ok {
		payload.StateBefore = stateBefore
	}
	if stateAfter, ok := data["state_after"].(map[string]any); ok {
		payload.StateAfter = stateAfter
	}
}

// generateEventID creates a unique event identifier.
func generateEventID() string {
	return generateRunID() // Reuse the same random ID generation
}

// Additional event types for new features
const (
	EventGraphStart    EventType = "graph_start"
	EventGraphComplete EventType = "graph_complete"
	EventGraphError    EventType = "graph_error"

	EventNodeQueued EventType = "node_queued"

	EventCheckpointLoad  EventType = "checkpoint_load"
	EventCheckpointError EventType = "checkpoint_error"

	EventResume EventType = "resume"

	EventModelStart    EventType = "model_start"
	EventModelComplete EventType = "model_complete"
	EventModelError    EventType = "model_error"

	EventToolStart    EventType = "tool_start"
	EventToolComplete EventType = "tool_complete"
	EventToolError    EventType = "tool_error"
)

// NodeStatus represents the visual state of a node during execution.
type NodeStatus string

// Node status constants
const (
	NodeStatusIdle      NodeStatus = "idle"      // Not yet executed
	NodeStatusQueued    NodeStatus = "queued"    // Queued for execution
	NodeStatusActive    NodeStatus = "active"    // Currently executing
	NodeStatusCompleted NodeStatus = "completed" // Execution completed successfully
	NodeStatusPaused    NodeStatus = "paused"    // Paused at breakpoint or interrupt
	NodeStatusError     NodeStatus = "error"     // Execution failed
	NodeStatusSkipped   NodeStatus = "skipped"   // Skipped (not in execution path)
)

// EdgeTraversal represents an edge being traversed during execution.
type EdgeTraversal struct {
	From      string    `json:"from"`                 // Source node
	To        string    `json:"to"`                   // Target node
	Timestamp time.Time `json:"timestamp"`            // When the edge was traversed
	Superstep int       `json:"superstep"`            // Superstep number
	MessageID string    `json:"message_id,omitempty"` // Optional message ID
}

// GraphSnapshot represents the current state of the graph visualization.
type GraphSnapshot struct {
	RunID     string    `json:"run_id"`
	Superstep int       `json:"superstep"`
	Timestamp time.Time `json:"timestamp"`

	// Node states
	NodeStates     map[string]NodeStatus `json:"node_states"`     // node_name -> status
	ActiveNodes    []string              `json:"active_nodes"`    // Currently executing
	CompletedNodes []string              `json:"completed_nodes"` // Finished
	PausedNodes    []string              `json:"paused_nodes"`    // Paused
	ErrorNodes     []string              `json:"error_nodes"`     // Failed

	// Edge activity
	RecentEdges []EdgeTraversal `json:"recent_edges"` // Recently traversed edges
	ActiveEdges []EdgeTraversal `json:"active_edges"` // Currently active

	// Execution position
	CurrentNode   string   `json:"current_node,omitempty"`
	ExecutionPath []string `json:"execution_path"` // Nodes executed so far

	// State information
	StateKeys []string `json:"state_keys"` // Available state keys
	StateSize int      `json:"state_size"` // State size in bytes
}

// GraphStateUpdate represents an incremental graph state update.
// Used for efficient WebSocket broadcasting of graph changes.
type GraphStateUpdate struct {
	RunID     string    `json:"run_id"`
	Timestamp time.Time `json:"timestamp"`
	Superstep int       `json:"superstep"`

	// What changed
	UpdateType   string         `json:"update_type"` // node_activated, node_completed, edge_traversed, etc.
	Node         string         `json:"node,omitempty"`
	NodeStatus   NodeStatus     `json:"node_status,omitempty"`
	Edge         *EdgeTraversal `json:"edge,omitempty"`
	StateChanged []string       `json:"state_changed,omitempty"` // State keys that changed
}
