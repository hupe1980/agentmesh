package graph

import (
	"github.com/hupe1980/agentmesh/pkg/state"
	"context"
	"fmt"
	"iter"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// SimpleGraphExecutor executes nodes sequentially in topological order.
// Unlike Pregel BSP execution, this executor:
//   - Runs nodes one at a time (single-threaded)
//   - Follows edges in order (no parallel execution)
//   - Easier to debug and understand execution flow
//   - Lower overhead (no message bus, no synchronization barriers)
//
// Use Cases:
//   - Debugging graph logic
//   - Understanding execution order
//   - Simple workflows that don't need parallelism
//   - Testing without Pregel complexity
type SimpleGraphExecutor struct {
	topology     *ExecutorTopology
	stateManager StateManager
	currentStep  atomic.Int64
	paused       map[string]bool
}

// NewSimpleGraphExecutor creates a sequential executor for the given topology.
func NewSimpleGraphExecutor() Executor {
	return &SimpleGraphExecutor{
		paused: make(map[string]bool),
	}
}

// Run executes the graph sequentially, node by node.
func (e *SimpleGraphExecutor) Run(
	ctx context.Context,
	topology *ExecutorTopology,
	stateManager StateManager,
	initialMessages []message.Message,
	options *RunOptions,
) iter.Seq2[state.ExecutionResult, error] {
	e.topology = topology
	e.stateManager = stateManager
	e.currentStep.Store(0)

	return func(yield func(state.ExecutionResult, error) bool) {
		// Initialize state with starting messages
		runID := options.RunID
		if runID == "" {
			runID = uuid.New().String()
		}

		// Add initial messages to state
		events := make([]state.ExecutionResult, len(initialMessages))
		for i, msg := range initialMessages {
			events[i] = state.ExecutionResult{
				Message:   msg,
				ID:        uuid.New().String(),
				GraphID:   runID,
				Node:      topology.StartKey,
				Timestamp: time.Now(),
			}
		}

		if len(events) > 0 {
			stateManager.AddMessages(events)
		}

		// Start execution from START node
		if err := e.executeFromNode(ctx, topology.StartKey, yield, options); err != nil {
			yield(state.ExecutionResult{}, err)
			return
		}
	}
}

// executeFromNode executes nodes starting from the given node.
func (e *SimpleGraphExecutor) executeFromNode(
	ctx context.Context,
	startNode string,
	yield func(state.ExecutionResult, error) bool,
	options *RunOptions,
) error {
	visited := make(map[string]bool)
	currentNode := startNode

	maxIterations := options.MaxIterations
	if maxIterations == 0 {
		maxIterations = 1000 // Default safety limit
	}

	iteration := 0
	for currentNode != e.topology.EndKey && currentNode != "" {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return err
		}

		// Check iteration limit
		iteration++
		if iteration > maxIterations {
			return fmt.Errorf("max iterations (%d) exceeded", maxIterations)
		}

		// Check for cycles
		if visited[currentNode] {
			return fmt.Errorf("cycle detected at node %s", currentNode)
		}
		visited[currentNode] = true

		// Check if paused
		if e.IsPaused(currentNode) {
			return fmt.Errorf("execution paused at node %s", currentNode)
		}

		// Execute the node
		node := e.topology.Nodes[currentNode]
		if node == nil {
			return fmt.Errorf("node %s not found", currentNode)
		}

		// Execute node and get result
		result, err := e.executeNode(ctx, node)
		if err != nil {
			event := state.ExecutionResult{
				ID:        uuid.New().String(),
				GraphID:   options.RunID,
				Node:      currentNode,
				Timestamp: time.Now(),
				Err:       err,
			}
			if !yield(event, err) {
				return nil // Consumer stopped iteration
			}
			return err
		}

		// Emit event for successful execution
		event := state.ExecutionResult{
			ID:        uuid.New().String(),
			GraphID:   options.RunID,
			Node:      currentNode,
			Timestamp: time.Now(),
			Updates:   result.Updates,
		}

		// Add result messages to event and state
		if len(result.Messages) > 0 {
			// Take the last message for the event
			event.Message = result.Messages[len(result.Messages)-1]

			// Add all messages to state
			msgEvents := make([]state.ExecutionResult, len(result.Messages))
			for i, msg := range result.Messages {
				msgEvents[i] = state.ExecutionResult{
					Message:   msg,
					ID:        uuid.New().String(),
					GraphID:   options.RunID,
					Node:      currentNode,
					Timestamp: time.Now(),
					Updates:   result.Updates,
				}
			}
			e.stateManager.AddMessages(msgEvents)
		}

		// Yield the event
		if !yield(event, nil) {
			return nil // Consumer stopped iteration
		}

		// Increment step counter
		e.currentStep.Add(1)

		// Determine next node
		currentNode = e.determineNextNode(currentNode, result)
	}

	return nil
}

// executeNode runs a single node and returns its result.
func (e *SimpleGraphExecutor) executeNode(ctx context.Context, node *Node) (*NodeResult, error) {
	// Create state writer for the node
	stateWriter := NewStateWriterAdapter(e.stateManager)

	// Execute node function
	result, err := node.RunFunc(ctx, stateWriter)
	if err != nil {
		return nil, fmt.Errorf("node %s failed: %w", node.Name, err)
	}

	// Apply state updates
	if len(result.Updates) > 0 {
		if err := e.stateManager.UpdateChannels(ctx, result.Updates); err != nil {
			return nil, fmt.Errorf("failed to update state: %w", err)
		}
	}

	return result, nil
}

// determineNextNode figures out which node to execute next based on edges.
func (e *SimpleGraphExecutor) determineNextNode(currentNode string, _ *NodeResult) string {
	// Check conditional edges first
	if conditionals, ok := e.topology.ConditionalByFrom[currentNode]; ok && len(conditionals) > 0 {
		// Use the first conditional edge set (simplified logic)
		cond := conditionals[0]

		// Evaluate condition to determine next node
		stateReader := NewStateReaderAdapter(e.stateManager)
		nextNodes := cond.Condition(context.Background(), stateReader)

		// Take the first returned node
		if len(nextNodes) > 0 {
			nextNode := nextNodes[0]
			// Validate next node exists
			if nextNode != "" && e.topology.Nodes[nextNode] != nil {
				return nextNode
			}
		}

		// If conditional returned empty/invalid, go to END
		return e.topology.EndKey
	}

	// Check regular outgoing edges
	outgoing := e.topology.Outgoing[currentNode]
	if len(outgoing) > 0 {
		// Take first outgoing edge (simplified - no parallel branches)
		return outgoing[0]
	}

	// No outgoing edges - go to END
	return e.topology.EndKey
}

// CurrentSuperstep returns the current step number.
func (e *SimpleGraphExecutor) CurrentSuperstep() int64 {
	return e.currentStep.Load()
}

// Pause marks a node to pause before execution.
func (e *SimpleGraphExecutor) Pause(nodeName string) {
	e.paused[nodeName] = true
}

// Resume clears the pause state for a node.
func (e *SimpleGraphExecutor) Resume(nodeName string) {
	delete(e.paused, nodeName)
}

// IsPaused checks if a node is currently paused.
func (e *SimpleGraphExecutor) IsPaused(nodeName string) bool {
	return e.paused[nodeName]
}

// Verify SimpleGraphExecutor implements Executor.
var _ Executor = (*SimpleGraphExecutor)(nil)
