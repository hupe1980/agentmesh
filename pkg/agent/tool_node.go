package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// toolNodeOptions holds configuration for a tool node.
type toolNodeOptions struct {
	nodeName        string
	errorPrefix     string
	continueOnError bool
	callbacks       *callbacks.Manager
}

// ToolNodeOption configures a tool node.
type ToolNodeOption func(*toolNodeOptions)

// WithToolNodeName sets the name of the tool node (default: "tool").
func WithToolNodeName(name string) ToolNodeOption {
	return func(c *toolNodeOptions) {
		c.nodeName = name
	}
}

// WithToolErrorPrefix sets the error message prefix (default: "tool node").
func WithToolErrorPrefix(prefix string) ToolNodeOption {
	return func(c *toolNodeOptions) {
		c.errorPrefix = prefix
	}
}

// WithContinueOnToolError configures whether to continue execution when a tool fails.
// If true, tool errors are returned as ToolMessages instead of stopping execution.
func WithContinueOnToolError(continueOnError bool) ToolNodeOption {
	return func(c *toolNodeOptions) {
		c.continueOnError = continueOnError
	}
}

// WithToolCallbacks sets the callback manager for the tool node.
// Callbacks enable intercepting and modifying tool invocations for access control,
// caching, metrics, error handling, and other cross-cutting concerns.
func WithToolCallbacks(cb *callbacks.Manager) ToolNodeOption {
	return func(c *toolNodeOptions) {
		c.callbacks = cb
	}
}

// formatToolResult converts a tool result (any type) to a string for ToolMessage.
func formatToolResult(result any) string {
	switch v := result.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		payload, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(payload)
	}
}

// ToolNode creates a reusable node that executes tool calls from the last AI message.
// It extracts tool calls from the most recent AIMessage, executes each tool,
// and returns the results as ToolMessages.
//
// Example:
//
//	g.AddNode(ToolNode(toolRegistry,
//	    WithToolNodeName("tools"),
//	    WithToolErrorPrefix("my agent"),
//	    WithContinueOnToolError(true)))
//
//nolint:gocyclo // Tool node requires handling many event types and configurations
func ToolNode(toolRegistry map[string]tool.Tool, opts ...ToolNodeOption) *graph.Node {
	config := toolNodeOptions{
		nodeName:        "tool",
		errorPrefix:     "tool node",
		continueOnError: false,
		callbacks:       nil,
	}

	for _, opt := range opts {
		opt(&config)
	}

	return &graph.Node{
		Name: config.nodeName,
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			transcript := s.MessagesSnapshot()
			if len(transcript) == 0 {
				return &graph.NodeResult{Updates: map[string]any{}}, nil
			}

			last := transcript[len(transcript)-1]
			ai, ok := last.(*message.AIMessage)
			if !ok || ai == nil {
				return &graph.NodeResult{Updates: map[string]any{}}, nil
			}

			if len(ai.ToolCalls) == 0 {
				return &graph.NodeResult{Updates: map[string]any{}}, nil
			}

			toolMessages := make([]message.Message, 0, len(ai.ToolCalls))
			for idx, call := range ai.ToolCalls {
				// Execute BeforeTool callbacks
				if config.callbacks != nil && config.callbacks.HasBeforeToolCallbacks() {
					callbackResult, err := config.callbacks.ExecuteBeforeTool(ctx, s, call)
					if err != nil {
						if config.continueOnError {
							toolCallID := call.ID
							if toolCallID == "" {
								toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
							}
							errMsg := fmt.Sprintf("Error: callback rejected: %v", err)
							toolMessages = append(toolMessages, message.NewToolMessage(toolCallID, errMsg))
							continue
						}
						return nil, fmt.Errorf("%s: before tool callback: %w", config.errorPrefix, err)
					}
					if callbackResult != nil {
						// Short-circuit: use callback result instead of calling tool
						toolCallID := call.ID
						if toolCallID == "" {
							toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
						}
						text := formatToolResult(callbackResult)
						toolMessages = append(toolMessages, message.NewToolMessage(toolCallID, text))
						continue
					}
				}

				tool := toolRegistry[call.Name]
				if tool == nil {
					if config.continueOnError {
						toolCallID := call.ID
						if toolCallID == "" {
							toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
						}
						errMsg := fmt.Sprintf("Error: tool %q not registered", call.Name)
						toolMessages = append(toolMessages, message.NewToolMessage(toolCallID, errMsg))
						continue
					}
					return nil, fmt.Errorf("%s: tool %q not registered", config.errorPrefix, call.Name)
				}

				args := "{}"
				if len(call.Arguments) > 0 {
					payload, err := json.Marshal(call.Arguments)
					if err != nil {
						if config.continueOnError {
							toolCallID := call.ID
							if toolCallID == "" {
								toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
							}
							errMsg := fmt.Sprintf("Error: failed to marshal arguments: %v", err)
							toolMessages = append(toolMessages, message.NewToolMessage(toolCallID, errMsg))
							continue
						}
						return nil, fmt.Errorf("%s: marshal arguments for tool %q: %w", config.errorPrefix, call.Name, err)
					}
					args = string(payload)
				}

				result, err := tool.Call(ctx, args)
				if err != nil {
					// Execute OnToolError callbacks
					if config.callbacks != nil && config.callbacks.HasOnToolErrorCallbacks() {
						fallback, cbErr := config.callbacks.ExecuteOnToolError(ctx, s, call, err)
						if cbErr != nil {
							err = cbErr // Use transformed error
						}
						if fallback != nil {
							// Callback provided fallback result
							result = fallback
							err = nil // Clear error since we have fallback
						}
					}

					// If still error after callbacks, handle it
					if err != nil {
						if config.continueOnError {
							toolCallID := call.ID
							if toolCallID == "" {
								toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
							}
							errMsg := fmt.Sprintf("Error: %v", err)
							toolMessages = append(toolMessages, message.NewToolMessage(toolCallID, errMsg))
							continue
						}
						return nil, fmt.Errorf("%s: tool %q call failed: %w", config.errorPrefix, call.Name, err)
					}
				}

				// Execute AfterTool callbacks
				if config.callbacks != nil && config.callbacks.HasAfterToolCallbacks() {
					transformed, err := config.callbacks.ExecuteAfterTool(ctx, s, call, result)
					if err != nil {
						if config.continueOnError {
							toolCallID := call.ID
							if toolCallID == "" {
								toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
							}
							errMsg := fmt.Sprintf("Error: callback failed: %v", err)
							toolMessages = append(toolMessages, message.NewToolMessage(toolCallID, errMsg))
							continue
						}
						return nil, fmt.Errorf("%s: after tool callback: %w", config.errorPrefix, err)
					}
					if transformed != nil {
						result = transformed
					}
				}

				text := formatToolResult(result)

				toolCallID := call.ID
				if toolCallID == "" {
					toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
				}

				toolMessages = append(toolMessages, message.NewToolMessage(toolCallID, text))
			}

			return &graph.NodeResult{
				Messages: toolMessages,
				Updates:  map[string]any{},
			}, nil
		},
	}
}
