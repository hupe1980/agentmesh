package a2a

import (
	"context"
	"fmt"

	a2atypes "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Executor wraps an AgentMesh CompiledGraph as an A2A AgentExecutor.
// It implements the a2asrv.AgentExecutor interface, allowing AgentMesh
// graphs to be exposed as A2A-compliant services.
type Executor struct {
	compiled *graph.CompiledGraph
}

// NewExecutor creates a new A2A executor that wraps an AgentMesh compiled graph.
func NewExecutor(compiled *graph.CompiledGraph) *Executor {
	return &Executor{compiled: compiled}
}

// Execute implements a2asrv.AgentExecutor.Execute.
// It converts the incoming A2A message to AgentMesh format, executes the graph,
// and writes the results back to the A2A event queue.
func (e *Executor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, q eventqueue.Queue) error {
	// Convert A2A message to AgentMesh messages
	messages, err := ConvertFromA2AMessage(reqCtx.Message)
	if err != nil {
		return fmt.Errorf("failed to convert A2A message: %w", err)
	}

	// Execute the graph
	results, err := e.compiled.Invoke(ctx, messages)
	if err != nil {
		return fmt.Errorf("graph execution failed: %w", err)
	}

	// Convert results back to A2A format and write to queue
	for _, msg := range results {
		a2aMsg, err := ConvertToA2AMessage(msg)
		if err != nil {
			return fmt.Errorf("failed to convert result message: %w", err)
		}

		// Write the message to the queue
		if err := q.Write(ctx, a2aMsg); err != nil {
			return fmt.Errorf("failed to write message to queue: %w", err)
		}
	}

	return nil
}

// Cancel implements a2asrv.AgentExecutor.Cancel.
// Currently a no-op as AgentMesh doesn't have built-in cancellation for running graphs.
// Future implementations could add context cancellation support.
func (e *Executor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, q eventqueue.Queue) error {
	// AgentMesh doesn't currently support cancellation of running graphs
	// This could be enhanced in the future with context cancellation
	return nil
}

// StreamingExecutor wraps an AgentMesh CompiledGraph with streaming support.
type StreamingExecutor struct {
	compiled *graph.CompiledGraph
}

// NewStreamingExecutor creates a new streaming A2A executor.
func NewStreamingExecutor(compiled *graph.CompiledGraph) *StreamingExecutor {
	return &StreamingExecutor{compiled: compiled}
}

// Execute implements a2asrv.AgentExecutor.Execute with streaming support.
func (e *StreamingExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, q eventqueue.Queue) error {
	// Convert A2A message to AgentMesh messages
	messages, err := ConvertFromA2AMessage(reqCtx.Message)
	if err != nil {
		return fmt.Errorf("failed to convert A2A message: %w", err)
	}

	// Stream execution results
	stream, err := e.compiled.Stream(ctx, messages)
	if err != nil {
		return fmt.Errorf("failed to start streaming: %w", err)
	}

	// Process each event from the stream
	for stream.Next() {
		event := stream.Current()

		if event.Err != nil {
			return fmt.Errorf("streaming error: %w", event.Err)
		}

		// Convert the message if present
		if len(event.Messages) > 0 {
			for _, msg := range event.Messages {
				a2aMsg, err := ConvertToA2AMessage(msg)
				if err != nil {
					return fmt.Errorf("failed to convert streamed message: %w", err)
				}

				if err := q.Write(ctx, a2aMsg); err != nil {
					return fmt.Errorf("failed to write streamed message: %w", err)
				}
			}
		}
	}

	// Check for any errors after streaming completes
	if err := stream.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

// Cancel implements a2asrv.AgentExecutor.Cancel for streaming.
func (e *StreamingExecutor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, q eventqueue.Queue) error {
	return nil
}

// CreateAgentCard creates an A2A AgentCard from an AgentMesh graph configuration.
func CreateAgentCard(name, description, url string, skills []a2atypes.AgentSkill) *a2atypes.AgentCard {
	return &a2atypes.AgentCard{
		Name:               name,
		Description:        description,
		URL:                url,
		PreferredTransport: a2atypes.TransportProtocolGRPC,
		DefaultInputModes:  []string{"text/plain", "application/json"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
		Capabilities: a2atypes.AgentCapabilities{
			Streaming:              true,
			PushNotifications:      false,
			StateTransitionHistory: false,
		},
		Skills: skills,
	}
}

// CreateAgentSkill creates an A2A AgentSkill for an AgentMesh capability.
func CreateAgentSkill(id, name, description string, tags []string, examples []string) a2atypes.AgentSkill {
	return a2atypes.AgentSkill{
		ID:          id,
		Name:        name,
		Description: description,
		Tags:        tags,
		Examples:    examples,
		InputModes:  []string{"text/plain", "application/json"},
		OutputModes: []string{"text/plain", "application/json"},
	}
}
