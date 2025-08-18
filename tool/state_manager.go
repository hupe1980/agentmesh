package tool

import (
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/core"
)

// StateManagerTool provides tools for managing session state through ToolContext.
//
// This tool demonstrates how to use ToolContext for state management, agent flow control,
// and other framework integration capabilities. It provides a set of operations that
// tools can use to interact with the AgentMesh framework infrastructure.
type StateManagerTool struct {
	name        string
	description string
}

// NewStateManagerTool creates a new state management tool.
//
// This tool provides operations for:
//   - Reading and writing session state
//   - Agent flow control (transfer, escalate)
//   - Memory management
//   - Artifact handling
//
// Returns a fully initialized StateManagerTool that implements the Tool interface.
func NewStateManagerTool() *StateManagerTool {
	return &StateManagerTool{
		name: "state_manager",
		description: "Manages session state, agent flow control, and framework integration. " +
			"Supports operations: get_state, set_state, transfer_agent, escalate, save_artifact, " +
			"load_artifact, search_memory, store_memory.",
	}
}

// Name returns the tool identifier.
func (t *StateManagerTool) Name() string {
	return t.name
}

// Description returns the tool description.
func (t *StateManagerTool) Description() string {
	return t.description
}

// Parameters returns the JSON schema for tool parameters.
func (t *StateManagerTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type": "string",
				"enum": []string{
					"get_state", "set_state", "transfer_agent", "escalate",
					"save_artifact", "load_artifact", "search_memory", "store_memory",
					"list_artifacts", "get_session_history", "skip_summarization",
				},
				"description": "The state management operation to perform",
			},
			"key": map[string]interface{}{
				"type":        "string",
				"description": "State key for get_state/set_state operations",
			},
			"value": map[string]interface{}{
				"description": "Value for set_state operations (any type)",
			},
			"agent_name": map[string]interface{}{
				"type":        "string",
				"description": "Agent name for transfer_agent operation",
			},
			"artifact_id": map[string]interface{}{
				"type":        "string",
				"description": "Artifact identifier for artifact operations",
			},
			"data": map[string]interface{}{
				"type":        "string",
				"description": "Base64 encoded data for save_artifact operation",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query for memory operations",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to store in memory",
			},
			"metadata": map[string]interface{}{
				"type":        "object",
				"description": "Metadata for memory storage",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Limit for search operations (default: 10)",
				"default":     10,
			},
		},
		"required": []string{"operation"},
	}
}

// Call implements the Tool interface with structured arguments.
func (t *StateManagerTool) Call(toolCtx *core.ToolContext, args map[string]interface{}) (interface{}, error) {
	operation, ok := args["operation"].(string)
	if !ok {
		return nil, fmt.Errorf("operation parameter is required")
	}

	switch operation {
	case "get_state":
		return t.handleGetState(args, toolCtx)
	case "set_state":
		return t.handleSetState(args, toolCtx)
	case "transfer_agent":
		return t.handleTransferAgent(args, toolCtx)
	case "escalate":
		return t.handleEscalate(args, toolCtx)
	case "save_artifact":
		return t.handleSaveArtifact(args, toolCtx)
	case "load_artifact":
		return t.handleLoadArtifact(args, toolCtx)
	case "search_memory":
		return t.handleSearchMemory(args, toolCtx)
	case "store_memory":
		return t.handleStoreMemory(args, toolCtx)
	case "list_artifacts":
		return t.handleListArtifacts(args, toolCtx)
	case "get_session_history":
		return t.handleGetSessionHistory(args, toolCtx)
	case "skip_summarization":
		return t.handleSkipSummarization(args, toolCtx)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

// handleGetState retrieves a value from session state.
func (t *StateManagerTool) handleGetState(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	key, ok := args["key"].(string)
	if !ok {
		return nil, fmt.Errorf("key parameter is required for get_state operation")
	}

	value, exists := toolCtx.GetState(key)
	if !exists {
		return map[string]interface{}{
			"key":    key,
			"exists": false,
			"value":  nil,
		}, nil
	}

	return map[string]interface{}{
		"key":    key,
		"exists": true,
		"value":  value,
	}, nil
}

// handleSetState sets a value in session state.
func (t *StateManagerTool) handleSetState(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	key, ok := args["key"].(string)
	if !ok {
		return nil, fmt.Errorf("key parameter is required for set_state operation")
	}

	value := args["value"] // Can be any type

	toolCtx.SetState(key, value)

	return map[string]interface{}{
		"key":     key,
		"value":   value,
		"success": true,
		"message": fmt.Sprintf("State key '%s' set successfully", key),
	}, nil
}

// handleTransferAgent initiates agent transfer.
func (t *StateManagerTool) handleTransferAgent(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	agentName, ok := args["agent_name"].(string)
	if !ok {
		return nil, fmt.Errorf("agent_name parameter is required for transfer_agent operation")
	}

	toolCtx.TransferToAgent(agentName)

	return map[string]interface{}{
		"agent_name": agentName,
		"success":    true,
		"message":    fmt.Sprintf("Transfer to agent '%s' initiated", agentName),
	}, nil
}

// handleEscalate initiates escalation.
func (t *StateManagerTool) handleEscalate(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	toolCtx.Escalate()

	return map[string]interface{}{
		"success": true,
		"message": "Escalation initiated",
	}, nil
}

// handleSaveArtifact saves data as an artifact.
func (t *StateManagerTool) handleSaveArtifact(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	artifactID, ok := args["artifact_id"].(string)
	if !ok {
		return nil, fmt.Errorf("artifact_id parameter is required for save_artifact operation")
	}

	dataStr, ok := args["data"].(string)
	if !ok {
		return nil, fmt.Errorf("data parameter is required for save_artifact operation")
	}

	// For simplicity, treat data as plain text. In a real implementation,
	// you might want to support base64 encoding for binary data.
	data := []byte(dataStr)

	if err := toolCtx.SaveArtifact(artifactID, data); err != nil {
		return nil, fmt.Errorf("failed to save artifact: %w", err)
	}

	return map[string]interface{}{
		"artifact_id": artifactID,
		"size":        len(data),
		"success":     true,
		"message":     fmt.Sprintf("Artifact '%s' saved successfully", artifactID),
	}, nil
}

// handleLoadArtifact loads data from an artifact.
func (t *StateManagerTool) handleLoadArtifact(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	artifactID, ok := args["artifact_id"].(string)
	if !ok {
		return nil, fmt.Errorf("artifact_id parameter is required for load_artifact operation")
	}

	data, err := toolCtx.LoadArtifact(artifactID)
	if err != nil {
		return nil, fmt.Errorf("failed to load artifact: %w", err)
	}

	return map[string]interface{}{
		"artifact_id": artifactID,
		"data":        string(data),
		"size":        len(data),
		"success":     true,
	}, nil
}

// handleSearchMemory searches for relevant memories.
func (t *StateManagerTool) handleSearchMemory(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query parameter is required for search_memory operation")
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	results, err := toolCtx.SearchMemory(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search memory: %w", err)
	}

	return map[string]interface{}{
		"query":   query,
		"limit":   limit,
		"count":   len(results),
		"results": results,
		"success": true,
	}, nil
}

// handleStoreMemory stores content in memory.
func (t *StateManagerTool) handleStoreMemory(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	content, ok := args["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content parameter is required for store_memory operation")
	}

	metadata := make(map[string]interface{})
	if m, ok := args["metadata"].(map[string]interface{}); ok {
		metadata = m
	}

	if err := toolCtx.StoreMemory(content, metadata); err != nil {
		return nil, fmt.Errorf("failed to store memory: %w", err)
	}

	return map[string]interface{}{
		"content":  content,
		"metadata": metadata,
		"success":  true,
		"message":  "Memory stored successfully",
	}, nil
}

// handleListArtifacts lists all artifacts in the session.
func (t *StateManagerTool) handleListArtifacts(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	artifacts, err := toolCtx.ListArtifacts()
	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts: %w", err)
	}

	return map[string]interface{}{
		"artifacts": artifacts,
		"count":     len(artifacts),
		"success":   true,
	}, nil
}

// handleGetSessionHistory retrieves session history.
func (t *StateManagerTool) handleGetSessionHistory(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	history := toolCtx.GetSessionHistory()

	// Convert events to a more readable format
	events := make([]map[string]interface{}, len(history))
	for i, ev := range history {
		events[i] = map[string]interface{}{
			"id":          ev.ID,
			"author":      ev.Author,
			"timestamp":   ev.Timestamp,
			"partial":     ev.Partial,
			"has_content": ev.Content != nil,
		}
		if ev.Content != nil && len(ev.Content.Parts) > 0 {
			// Add a summary of content
			var contentSummary []string
			for _, part := range ev.Content.Parts {
				switch p := part.(type) {
				case core.TextPart:
					preview := p.Text
					if len(preview) > 100 {
						preview = preview[:100] + "..."
					}
					contentSummary = append(contentSummary, fmt.Sprintf("text: %s", preview))
				case core.FunctionCallPart:
					contentSummary = append(contentSummary, fmt.Sprintf("function_call: %s", p.FunctionCall.Name))
				case core.FunctionResponsePart:
					contentSummary = append(contentSummary, fmt.Sprintf("function_response: %s", p.FunctionResponse.Name))
				default:
					contentSummary = append(contentSummary, "other")
				}
			}
			events[i]["content_summary"] = strings.Join(contentSummary, ", ")
		}
	}

	return map[string]interface{}{
		"events":  events,
		"count":   len(events),
		"success": true,
	}, nil
}

// handleSkipSummarization sets the skip summarization flag.
func (t *StateManagerTool) handleSkipSummarization(args map[string]interface{}, toolCtx *core.ToolContext) (interface{}, error) {
	toolCtx.SkipSummarization()

	return map[string]interface{}{
		"success": true,
		"message": "Summarization will be skipped for this interaction",
	}, nil
}
