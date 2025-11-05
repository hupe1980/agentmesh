package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// toolNodeOptions holds configuration for a tool node.
type toolNodeOptions struct {
	nodeName        string
	errorPrefix     string
	continueOnError bool
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
func ToolNode(toolRegistry map[string]tool.Tool, opts ...ToolNodeOption) *graph.Node {
	config := toolNodeOptions{
		nodeName:        "tool",
		errorPrefix:     "tool node",
		continueOnError: false,
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

				var text string
				switch v := result.(type) {
				case nil:
					text = ""
				case string:
					text = v
				default:
					payload, err := json.Marshal(v)
					if err != nil {
						text = fmt.Sprintf("%v", v)
					} else {
						text = string(payload)
					}
				}

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
