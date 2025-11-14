package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"

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
	callbacks       *callbacks.PluginManager
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

// WithToolCallbacks sets the plugin manager for the tool node.
// Plugins enable intercepting and modifying tool invocations for access control,
// caching, metrics, error handling, and other cross-cutting concerns.
func WithToolCallbacks(cb *callbacks.PluginManager) ToolNodeOption {
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

// marshalToolArguments converts tool call arguments to JSON string.
// Returns error messages if marshaling fails and continueOnError is true.
func marshalToolArguments(call message.ToolCall, config *toolNodeOptions, idx int) (string, []message.Message, error) {
	if len(call.Arguments) == 0 {
		return "{}", nil, nil
	}

	payload, err := json.Marshal(call.Arguments)
	if err != nil {
		if config.continueOnError {
			toolCallID := call.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
			}
			errMsg := fmt.Sprintf("Error: failed to marshal arguments: %v", err)
			return "", []message.Message{message.NewToolMessage(toolCallID, errMsg)}, nil
		}
		return "", nil, fmt.Errorf("%s: marshal arguments for tool %q: %w", config.errorPrefix, call.Name, err)
	}

	return string(payload), nil, nil
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
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			events := s.MessagesSnapshot()
			if len(events) == 0 {
				return &graph.NodeResult{Updates: map[string]any{}}, nil
			}

			lastMsg := events[len(events)-1].Message
			ai, ok := lastMsg.(*message.AIMessage)
			if !ok || ai == nil {
				return &graph.NodeResult{Updates: map[string]any{}}, nil
			}

			if len(ai.ToolCalls) == 0 {
				return &graph.NodeResult{Updates: map[string]any{}}, nil
			}

			toolMessages := make([]message.Message, 0, len(ai.ToolCalls))
			for idx, call := range ai.ToolCalls {
				// Execute BeforeTool callbacks
				msgs, err := handleBeforeToolCallback(ctx, s, call, &config, idx)
				if err != nil {
					return nil, err
				}
				if msgs != nil {
					toolMessages = append(toolMessages, msgs...)
					continue
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

				args, errMsgs, err := marshalToolArguments(call, &config, idx)
				if err != nil {
					return nil, err
				}
				if errMsgs != nil {
					toolMessages = append(toolMessages, errMsgs...)
					continue
				}

				result, err := tool.Call(ctx, args)
				if err != nil {
					errMsgs, handledErr := handleToolError(ctx, s, call, err, &config, idx)
					if handledErr != nil {
						return nil, handledErr
					}
					if errMsgs != nil {
						toolMessages = append(toolMessages, errMsgs...)
						continue
					}
				}

				// Execute AfterTool callbacks
				finalResult, callbackMsgs, err := handleAfterToolCallback(ctx, s, call, result, &config, idx)
				if err != nil {
					return nil, err
				}
				if callbackMsgs != nil {
					toolMessages = append(toolMessages, callbackMsgs...)
					continue
				}

				text := formatToolResult(finalResult)
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

// handleToolError processes tool execution errors with plugins and fallbacks.
// Returns error messages (if continuing on error) and error (nil if handled).
func handleToolError(ctx context.Context, _ state.Writer, call message.ToolCall, execErr error, config *toolNodeOptions, idx int) ([]message.Message, error) {
	err := execErr

	// Execute OnToolError plugins
	if config.callbacks != nil && config.callbacks.HasPlugins() {
		pluginErr := config.callbacks.ExecuteOnToolError(ctx, call.Name, err)
		if pluginErr != nil {
			err = pluginErr // Use transformed error
		}
	}

	// If still error after plugins, handle it
	if err != nil {
		if config.continueOnError {
			toolCallID := call.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
			}
			errMsg := fmt.Sprintf("Error: %v", err)
			toolMsg := message.NewToolMessage(toolCallID, errMsg)
			return []message.Message{toolMsg}, nil
		}
		return nil, fmt.Errorf("%s: tool %q call failed: %w", config.errorPrefix, call.Name, err)
	}

	return nil, nil
}

// handleBeforeToolCallback executes before-tool plugins.
// Returns error if plugin fails.
func handleBeforeToolCallback(ctx context.Context, _ state.Writer, call message.ToolCall, config *toolNodeOptions, idx int) ([]message.Message, error) {
	if config.callbacks == nil || !config.callbacks.HasPlugins() {
		return nil, nil
	}

	// Marshal arguments for plugin inspection
	var input any = call.Arguments
	if len(call.Arguments) > 0 {
		payload, _ := json.Marshal(call.Arguments)
		input = string(payload)
	}

	err := config.callbacks.ExecuteBeforeTool(ctx, call.Name, input)
	if err != nil {
		if config.continueOnError {
			toolCallID := call.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
			}
			errMsg := fmt.Sprintf("Error: plugin rejected: %v", err)
			return []message.Message{message.NewToolMessage(toolCallID, errMsg)}, nil
		}
		return nil, fmt.Errorf("%s: before tool plugin: %w", config.errorPrefix, err)
	}

	return nil, nil
}

// handleAfterToolCallback executes after-tool plugins.
// Returns error if plugin fails.
func handleAfterToolCallback(ctx context.Context, _ state.Writer, call message.ToolCall, result any, config *toolNodeOptions, idx int) (any, []message.Message, error) {
	if config.callbacks == nil || !config.callbacks.HasPlugins() {
		return result, nil, nil
	}

	pluginResult := callbacks.ToolResult{
		Output: result,
	}

	err := config.callbacks.ExecuteAfterTool(ctx, call.Name, pluginResult)
	if err != nil {
		if config.continueOnError {
			toolCallID := call.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
			}
			errMsg := fmt.Sprintf("Error: plugin failed: %v", err)
			return nil, []message.Message{message.NewToolMessage(toolCallID, errMsg)}, nil
		}
		return nil, nil, fmt.Errorf("%s: after tool plugin: %w", config.errorPrefix, err)
	}

	return result, nil, nil
}
