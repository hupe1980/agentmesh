package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// ToolNode executes tool calls from the last AI message.
// It extracts tool calls from the most recent AIMessage, executes each tool
// (sequentially or in parallel), and returns the results as ToolMessages.
type ToolNode struct {
	name            string
	toolRegistry    map[string]tool.Tool
	errorPrefix     string
	continueOnError bool
	parallel        bool
	callbacks       *callbacks.PluginManager
}

// ToolNodeOption configures a tool node.
type ToolNodeOption func(*ToolNode)

// WithToolNodeName sets the name of the tool node (default: "tool").
func WithToolNodeName(name string) ToolNodeOption {
	return func(n *ToolNode) {
		n.name = name
	}
}

// WithToolErrorPrefix sets the error message prefix (default: "tool node").
func WithToolErrorPrefix(prefix string) ToolNodeOption {
	return func(n *ToolNode) {
		n.errorPrefix = prefix
	}
}

// WithContinueOnToolError configures whether to continue execution when a tool fails.
// If true, tool errors are returned as ToolMessages instead of stopping execution.
func WithContinueOnToolError(continueOnError bool) ToolNodeOption {
	return func(n *ToolNode) {
		n.continueOnError = continueOnError
	}
}

// WithParallelToolExecution enables parallel execution of tool calls.
// When true, all tool calls from the AI message are executed concurrently.
// This can significantly improve performance when multiple independent tools are called.
func WithParallelToolExecution(parallel bool) ToolNodeOption {
	return func(n *ToolNode) {
		n.parallel = parallel
	}
}

// WithToolCallbacks sets the plugin manager for the tool node.
// Plugins enable intercepting and modifying tool invocations for access control,
// caching, metrics, error handling, and other cross-cutting concerns.
func WithToolCallbacks(cb *callbacks.PluginManager) ToolNodeOption {
	return func(n *ToolNode) {
		n.callbacks = cb
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
func marshalToolArguments(call message.ToolCall, node *ToolNode, idx int) (string, []message.Message, error) {
	if len(call.Arguments) == 0 {
		return "{}", nil, nil
	}

	payload, err := json.Marshal(call.Arguments)
	if err != nil {
		if node.continueOnError {
			toolCallID := call.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
			}
			errMsg := fmt.Sprintf("Error: failed to marshal arguments: %v", err)
			return "", []message.Message{message.NewToolMessage(toolCallID, errMsg)}, nil
		}
		return "", nil, fmt.Errorf("%s: marshal arguments for tool %q: %w", node.errorPrefix, call.Name, err)
	}

	return string(payload), nil, nil
}

// NewToolNode creates a new ToolNode with the given tool registry and options.
//
// Returns an error if the toolRegistry parameter is nil.
//
// Example:
//
//	node, err := NewToolNode(toolRegistry,
//	    WithToolNodeName("tools"),
//	    WithToolErrorPrefix("my agent"),
//	    WithContinueOnToolError(true),
//	    WithParallelToolExecution(true))
//	if err != nil {
//	    return err
//	}
//	g.AddNode(node)
func NewToolNode(toolRegistry map[string]tool.Tool, opts ...ToolNodeOption) (*ToolNode, error) {
	if toolRegistry == nil {
		return nil, fmt.Errorf("agent: toolRegistry cannot be nil")
	}

	node := &ToolNode{
		name:            "tool",
		toolRegistry:    toolRegistry,
		errorPrefix:     "tool node",
		continueOnError: false,
		parallel:        false,
		callbacks:       nil,
	}

	for _, opt := range opts {
		opt(node)
	}

	return node, nil
}

// Name returns the name of the tool node.
func (n *ToolNode) Name() string {
	return n.name
}

// Execute processes tool calls from the last AI message.
//
// Tool node requires handling many event types and configurations
func (n *ToolNode) Execute(ctx context.Context, view *state.ReadView) (state.Updates, error) {
	// Get last message from state
	lastMsg := LastMessage(view)
	if lastMsg == nil {
		return state.Updates{}, nil
	}

	ai, ok := lastMsg.(*message.AIMessage)
	if !ok || ai == nil {
		return state.Updates{}, nil
	}

	if len(ai.ToolCalls) == 0 {
		return state.Updates{}, nil
	}

	var toolMessages []message.Message
	var err error

	if n.parallel {
		// Parallel execution
		toolMessages, err = executeToolsParallel(ctx, view, ai.ToolCalls, n.toolRegistry, n)
	} else {
		// Sequential execution
		toolMessages, err = executeToolsSequential(ctx, view, ai.ToolCalls, n.toolRegistry, n)
	}

	if err != nil {
		return nil, err
	}

	builder := state.NewUpdateBuilder()
	state.AppendUpdate(builder, MessagesKey, toolMessages...)

	return builder.Build()
}

// handleToolError processes tool execution errors with plugins and fallbacks.
// Returns error messages (if continuing on error) and error (nil if handled).
func handleToolError(ctx context.Context, _ *state.ReadView, call message.ToolCall, execErr error, node *ToolNode, idx int) ([]message.Message, error) {
	err := execErr

	// Execute OnToolError plugins
	if node.callbacks != nil && node.callbacks.HasPlugins() {
		pluginErr := node.callbacks.ExecuteOnToolError(ctx, call.Name, err)
		if pluginErr != nil {
			err = pluginErr // Use transformed error
		}
	}

	// If still error after plugins, handle it
	if err != nil {
		if node.continueOnError {
			toolCallID := call.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
			}
			errMsg := fmt.Sprintf("Error: %v", err)
			toolMsg := message.NewToolMessage(toolCallID, errMsg)
			return []message.Message{toolMsg}, nil
		}
		return nil, fmt.Errorf("%s: tool %q call failed: %w", node.errorPrefix, call.Name, err)
	}

	return nil, nil
}

// handleBeforeToolCallback executes before-tool plugins.
// Returns error if plugin fails.
func handleBeforeToolCallback(ctx context.Context, _ *state.ReadView, call message.ToolCall, node *ToolNode, idx int) ([]message.Message, error) {
	if node.callbacks == nil || !node.callbacks.HasPlugins() {
		return nil, nil
	}

	// Marshal arguments for plugin inspection
	var input any = call.Arguments
	if len(call.Arguments) > 0 {
		payload, _ := json.Marshal(call.Arguments)
		input = string(payload)
	}

	err := node.callbacks.ExecuteBeforeTool(ctx, call.Name, input)
	if err != nil {
		if node.continueOnError {
			toolCallID := call.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
			}
			errMsg := fmt.Sprintf("Error: plugin rejected: %v", err)
			return []message.Message{message.NewToolMessage(toolCallID, errMsg)}, nil
		}
		return nil, fmt.Errorf("%s: before tool plugin: %w", node.errorPrefix, err)
	}

	return nil, nil
}

// handleAfterToolCallback executes after-tool plugins.
// Returns error if plugin fails.
func handleAfterToolCallback(ctx context.Context, _ *state.ReadView, call message.ToolCall, result any, node *ToolNode, idx int) (any, []message.Message, error) {
	if node.callbacks == nil || !node.callbacks.HasPlugins() {
		return result, nil, nil
	}

	pluginResult := callbacks.ToolResult{
		Output: result,
	}

	err := node.callbacks.ExecuteAfterTool(ctx, call.Name, pluginResult)
	if err != nil {
		if node.continueOnError {
			toolCallID := call.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
			}
			errMsg := fmt.Sprintf("Error: plugin failed: %v", err)
			return nil, []message.Message{message.NewToolMessage(toolCallID, errMsg)}, nil
		}
		return nil, nil, fmt.Errorf("%s: after tool plugin: %w", node.errorPrefix, err)
	}

	return result, nil, nil
}

// executeToolsSequential executes tool calls one by one in order.
func executeToolsSequential(ctx context.Context, view *state.ReadView, toolCalls []message.ToolCall, toolRegistry map[string]tool.Tool, node *ToolNode) ([]message.Message, error) {
	toolMessages := make([]message.Message, 0, len(toolCalls))

	for idx, call := range toolCalls {
		msgs, err := executeSingleTool(ctx, view, call, idx, toolRegistry, node)
		if err != nil {
			return nil, err
		}
		toolMessages = append(toolMessages, msgs...)
	}

	return toolMessages, nil
}

// executeToolsParallel executes tool calls concurrently using goroutines.
// Results are collected in the original order of tool calls.
func executeToolsParallel(ctx context.Context, view *state.ReadView, toolCalls []message.ToolCall, toolRegistry map[string]tool.Tool, node *ToolNode) ([]message.Message, error) {
	type result struct {
		idx      int
		messages []message.Message
		err      error
	}

	results := make(chan result, len(toolCalls))
	var wg sync.WaitGroup

	// Launch goroutines for each tool call
	for idx, call := range toolCalls {
		wg.Add(1)
		go func(i int, c message.ToolCall) {
			defer wg.Done()
			msgs, err := executeSingleTool(ctx, view, c, i, toolRegistry, node)
			results <- result{idx: i, messages: msgs, err: err}
		}(idx, call)
	}

	// Wait for all goroutines and close channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results in order
	resultMap := make(map[int]result)
	for r := range results {
		if r.err != nil && !node.continueOnError {
			return nil, r.err
		}
		resultMap[r.idx] = r
	}

	// Reconstruct messages in original order
	toolMessages := make([]message.Message, 0, len(toolCalls))
	for i := 0; i < len(toolCalls); i++ {
		if r, ok := resultMap[i]; ok {
			if r.err != nil {
				// Error was handled in continueOnError mode
				continue
			}
			toolMessages = append(toolMessages, r.messages...)
		}
	}

	return toolMessages, nil
}

// executeSingleTool executes a single tool call with all callbacks and error handling.
func executeSingleTool(ctx context.Context, view *state.ReadView, call message.ToolCall, idx int, toolRegistry map[string]tool.Tool, node *ToolNode) ([]message.Message, error) {
	// Execute BeforeTool callbacks
	msgs, err := handleBeforeToolCallback(ctx, view, call, node, idx)
	if err != nil {
		return nil, err
	}
	if msgs != nil {
		return msgs, nil
	}

	// Check if tool exists
	t := toolRegistry[call.Name]
	if t == nil {
		if node.continueOnError {
			toolCallID := call.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
			}
			errMsg := fmt.Sprintf("Error: tool %q not registered", call.Name)
			return []message.Message{message.NewToolMessage(toolCallID, errMsg)}, nil
		}
		return nil, fmt.Errorf("%s: tool %q not registered", node.errorPrefix, call.Name)
	}

	// Marshal arguments
	args, errMsgs, err := marshalToolArguments(call, node, idx)
	if err != nil {
		return nil, err
	}
	if errMsgs != nil {
		return errMsgs, nil
	}

	// Execute tool
	result, err := t.Call(ctx, args)
	if err != nil {
		errMsgs, handledErr := handleToolError(ctx, view, call, err, node, idx)
		if handledErr != nil {
			return nil, handledErr
		}
		if errMsgs != nil {
			return errMsgs, nil
		}
	}

	// Execute AfterTool callbacks
	finalResult, callbackMsgs, err := handleAfterToolCallback(ctx, view, call, result, node, idx)
	if err != nil {
		return nil, err
	}
	if callbackMsgs != nil {
		return callbackMsgs, nil
	}

	// Format result as ToolMessage
	text := formatToolResult(finalResult)
	toolCallID := call.ID
	if toolCallID == "" {
		toolCallID = fmt.Sprintf("%s-%d", call.Name, idx)
	}

	return []message.Message{message.NewToolMessage(toolCallID, text)}, nil
}
