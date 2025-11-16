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

		// Add initial messages to state
		if len(initialMessages) > 0 {
			events := make([]state.ExecutionResult, len(initialMessages))
			for i, msg := range initialMessages {
				events[i] = state.ExecutionResult{
					Message:   msg,
					ID:        uuid.New().String(),
					GraphID:   runID,
					Node:      compiled.StartNode,
					Timestamp: time.Now(),
				}
			}
			compiled.StateManager.AddMessages(events)
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

		// Execute the node
		result, err := node.Run(ctx, compiled.StateManager)
		if err != nil {
			event := state.ExecutionResult{
				ID:        uuid.New().String(),
				GraphID:   runID,
				Node:      nodeName,
				Timestamp: time.Now(),
				Err:       err,
			}
			if !yield(event, err) {
				return nil
			}
			return err
		}

		// Process node result
		if result != nil {
			// Update state
			if len(result.Updates) > 0 {
				for key, value := range result.Updates {
					if err := compiled.StateManager.Set(key, value); err != nil {
						return fmt.Errorf("failed to update state for key %q: %w", key, err)
					}
				}
			}

			// Add messages
			if len(result.Messages) > 0 {
				events := make([]state.ExecutionResult, len(result.Messages))
				for i, msg := range result.Messages {
					events[i] = state.ExecutionResult{
						Message:   msg,
						ID:        uuid.New().String(),
						GraphID:   runID,
						Node:      nodeName,
						Timestamp: time.Now(),
					}
				}
				compiled.StateManager.AddMessages(events)

				// Yield each message event
				for _, event := range events {
					if !yield(event, nil) {
						return nil
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
		for _, cond := range conditionals {
			targets := cond.Condition(ctx, compiled.StateManager)
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
