package exec

import (
	"context"
	"fmt"
	"iter"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Sequential is a simple sequential executor that runs nodes one at a time
// in topological order. This is useful for debugging and simple workflows.
type Sequential struct{}

// NewSequential creates a new sequential executor.
func NewSequential() *Sequential {
	return &Sequential{}
}

// Run executes the compiled graph sequentially.
func (s *Sequential) Run(
	ctx context.Context,
	compiled *compile.CompiledGraph,
	initialMessages []message.Message,
	opts ...graph.RunOption,
) iter.Seq2[state.ExecutionResult, error] {
	// Note: Sequential executor currently ignores checkpoint options
	_ = opts
	return func(yield func(state.ExecutionResult, error) bool) {
		runID := uuid.New().String()

		// Store initial messages in state
		if len(initialMessages) > 0 {
			updates := state.Updates{}
			state.AppendMessages(updates, initialMessages)
			if err := state.ApplyUpdates(ctx, compiled.Manager, updates); err != nil {
				yield(state.ExecutionResult{}, fmt.Errorf("failed to store initial messages: %w", err))
				return
			}
		}

		// Execute from start node
		if err := s.executeFromNode(ctx, compiled, compiled.StartNode, runID, yield); err != nil {
			yield(state.ExecutionResult{}, err)
		}
	}
}

// executeFromNode executes nodes starting from the given node.
//
//nolint:gocyclo,nestif // Sequential execution orchestration requires branching logic
func (s *Sequential) executeFromNode(
	ctx context.Context,
	compiled *compile.CompiledGraph,
	startNode string,
	runID string,
	yield func(state.ExecutionResult, error) bool,
) error {
	queue := []string{startNode}
	executionCount := 0
	maxExecutions := 1000 // Safety limit to prevent infinite loops

	for len(queue) > 0 {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return err
		}

		// Safety check to prevent infinite loops
		executionCount++
		if executionCount > maxExecutions {
			return fmt.Errorf("max execution count (%d) exceeded - possible infinite loop", maxExecutions)
		}

		// Pop node from queue
		nodeName := queue[0]
		queue = queue[1:]

		// Skip start/end nodes but follow their edges
		if nodeName == compiled.EndNode || nodeName == compiled.StartNode {
			nextNodes := s.findNextNodes(ctx, compiled, nodeName)
			queue = append(queue, nextNodes...)
			continue
		}

		// Get the node
		node := compiled.GetNode(nodeName)
		if node == nil {
			return fmt.Errorf("node %s not found", nodeName)
		}

		// Execute the node with current state read view
		view, err := compiled.Manager.CreateReadView(ctx)
		if err != nil {
			return fmt.Errorf("failed to create read view: %w", err)
		}
		result, err := node.Run(ctx, view)
		if err != nil {
			event := state.ExecutionResult{
				ID:        uuid.New().String(),
				GraphID:   runID,
				Node:      nodeName,
				Timestamp: time.Now(),
			}
			if !yield(event, err) {
				return nil
			}
			return err
		}

		// Process node result
		if result != nil {
			// Apply state updates
			if len(result.Updates) > 0 {
				if err := state.ApplyUpdates(ctx, compiled.Manager, result.Updates); err != nil {
					return fmt.Errorf("failed to apply state updates: %w", err)
				}

				// Extract messages from updates and yield as execution events
				if messagesAny, ok := result.Updates[state.MessagesKey.Name()]; ok {
					if messages, ok := messagesAny.([]message.Message); ok && len(messages) > 0 {
						// Yield events to caller
						for _, msg := range messages {
							event := state.ExecutionResult{
								Message:   msg,
								ID:        uuid.New().String(),
								GraphID:   runID,
								Node:      nodeName,
								Timestamp: time.Now(),
							}
							if !yield(event, nil) {
								return nil
							}
						}
					}
				}
			}
		}

		// Find next nodes to execute
		nextNodes := s.findNextNodes(ctx, compiled, nodeName)
		queue = append(queue, nextNodes...)
	}

	return nil
}

// findNextNodes determines which nodes should execute next based on edges and conditionals.
func (s *Sequential) findNextNodes(
	ctx context.Context,
	compiled *compile.CompiledGraph,
	nodeName string,
) []string {
	var next []string

	// Check for conditional edges
	if conditionals, ok := compiled.Topology.ConditionalByFrom[nodeName]; ok {
		view, err := compiled.Manager.CreateReadView(ctx)
		if err != nil {
			// If we can't create a read view, fall back to regular edges
			if outgoing, ok := compiled.Topology.Outgoing[nodeName]; ok {
				return outgoing
			}
			return nil
		}
		for _, cond := range conditionals {
			targets := cond.Condition(ctx, view)
			next = append(next, targets...)
		}
	} else {
		// Use regular outgoing edges
		if outgoing, ok := compiled.Topology.Outgoing[nodeName]; ok {
			next = append(next, outgoing...)
		}
	}

	return next
}
