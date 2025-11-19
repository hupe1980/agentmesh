package graph

import (
	"context"
	"fmt"
	"iter"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// SequentialExecutor is a simple sequential executor that runs nodes one at a time
// in topological order. This is useful for debugging and simple workflows.
type SequentialExecutor[I, O any] struct {
	// Generic executor configuration
	inputToState  func(I) state.Updates // Convert input to initial state
	outputKey     string                // Which state key to yield as output
	outputAdapter func(any) O           // Convert state value to output type
}

// NewSequentialExecutor creates a message-based Sequential executor (default behavior).
// This is for agent workflows, LLM chains, and conversational systems.
// Input: []message.Message, Output: message.Message (last message from messages key)
func NewSequentialExecutor() *SequentialExecutor[[]message.Message, message.Message] {
	return NewGenericSequentialExecutor[[]message.Message, message.Message](
		// Input: Convert []message.Message to state using standard messages key
		func(input []message.Message) state.Updates {
			if len(input) == 0 {
				return nil
			}
			return state.Updates{MessagesKeyName: input}
		},
		// Output: Watch standard messages key
		MessagesKeyName,
		// Output: Extract last message from messages list
		func(value any) message.Message {
			if msgs, ok := value.([]message.Message); ok && len(msgs) > 0 {
				return msgs[len(msgs)-1]
			}
			return nil
		},
	)
}

// NewStateSequentialExecutor creates a state-only Sequential executor.
// This is for pure state transformation workflows, data pipelines, ETL.
// Input: state.Updates, Output: state.Updates (all state updates)
func NewStateSequentialExecutor() *SequentialExecutor[state.Updates, state.Updates] {
	return NewGenericSequentialExecutor[state.Updates, state.Updates](
		// Input: Use provided state.Updates directly as initial state
		func(input state.Updates) state.Updates {
			return input
		},
		// Output: Special marker to yield all updates (not just one key)
		"*", // Wildcard means "yield entire state.Updates"
		// Output: Return updates as-is
		func(value any) state.Updates {
			if updates, ok := value.(state.Updates); ok {
				return updates
			}
			return nil
		},
	)
}

// NewKeySequentialExecutor creates a key-based Sequential executor.
// This is for domain-specific workflows with typed input/output.
// Input: type I stored in inputKey, Output: type O from outputKey
func NewKeySequentialExecutor[I, O any](
	inputKey *state.Key[I],
	outputKey *state.Key[O],
) *SequentialExecutor[I, O] {
	return NewGenericSequentialExecutor[I, O](
		// Input: Store input in specified key
		func(input I) state.Updates {
			return state.Updates{inputKey.Name(): input}
		},
		// Output: Watch specified key
		outputKey.Name(),
		// Output: Type-safe extraction
		func(value any) O {
			if typed, ok := value.(O); ok {
				return typed
			}
			var zero O
			return zero
		},
	)
}

// NewGenericSequentialExecutor creates a fully customizable Sequential executor.
// This is for advanced use cases with custom input/output transformations.
//
// Parameters:
//   - inputToState: Converts input I to initial state updates
//   - outputKey: Which state key to watch and yield (use "*" for all updates)
//   - outputAdapter: Converts state value to output type O
func NewGenericSequentialExecutor[I, O any](
	inputToState func(I) state.Updates,
	outputKey string,
	outputAdapter func(any) O,
) *SequentialExecutor[I, O] {
	return &SequentialExecutor[I, O]{
		inputToState:  inputToState,
		outputKey:     outputKey,
		outputAdapter: outputAdapter,
	}
}

// Run executes the compiled graph sequentially.
func (s *SequentialExecutor[I, O]) Run(
	ctx context.Context,
	compiled *Compiled[I, O],
	input I,
	opts ...RunOption,
) iter.Seq2[O, error] {
	// Note: Sequential executor currently ignores checkpoint options
	_ = opts
	return func(yield func(O, error) bool) {
		runID := uuid.New().String()
		_ = runID // For future use with checkpointing

		// Convert input to initial state using adapter
		var inputValue any = input
		if inputValue != nil {
			initialState := s.inputToState(input)
			if len(initialState) > 0 {
				if err := compiled.manager.ApplyUpdates(ctx, initialState); err != nil {
					var zero O
					yield(zero, fmt.Errorf("failed to apply initial state: %w", err))
					return
				}
			}
		}

		// Execute from start node
		if err := s.executeFromNode(ctx, compiled, StartNode, yield); err != nil {
			var zero O
			yield(zero, err)
		}
	}
}

// executeFromNode executes nodes starting from the given node.
//
//nolint:gocyclo,nestif // Sequential execution orchestration requires branching logic
func (s *SequentialExecutor[I, O]) executeFromNode(
	ctx context.Context,
	compiled *Compiled[I, O],
	startNode string,
	yield func(O, error) bool,
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
		if nodeName == EndNode || nodeName == StartNode {
			nextNodes := s.findNextNodes(ctx, compiled, nodeName)
			queue = append(queue, nextNodes...)
			continue
		}

		// Get the node
		node := compiled.graph.Nodes[nodeName]
		if node == nil {
			return fmt.Errorf("node %s not found", nodeName)
		}

		// Execute the node with current state read view
		view, err := compiled.manager.CreateReadView(ctx)
		if err != nil {
			return fmt.Errorf("failed to create read view: %w", err)
		}
		updates, err := node.Execute(ctx, view)
		if err != nil {
			// Yield error
			var zero O
			if !yield(zero, err) {
				return nil
			}
			return err
		}

		// Process node updates
		if updates != nil {
			// Apply state updates
			if len(updates) > 0 {
				if err := compiled.manager.ApplyUpdates(ctx, updates); err != nil {
					return fmt.Errorf("failed to apply state updates: %w", err)
				}

				// Extract output from updates based on configured key and yield
				if s.outputKey == "*" {
					// Yield entire state.Updates (for state-only executor)
					output := s.outputAdapter(updates)
					if !yield(output, nil) {
						return nil
					}
				} else if value, ok := updates[s.outputKey]; ok {
					// Yield specific key value
					output := s.outputAdapter(value)
					if !yield(output, nil) {
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
func (s *SequentialExecutor[I, O]) findNextNodes(
	ctx context.Context,
	compiled *Compiled[I, O],
	nodeName string,
) []string {
	var next []string

	// Check for conditional edges in the branches list
	for _, branch := range compiled.graph.Branches {
		if branch.From == nodeName {
			// Found conditional edges for this node
			view, err := compiled.manager.CreateReadView(ctx)
			if err != nil {
				// If we can't create a read view, fall back to regular edges
				if outgoing, ok := compiled.topology.outgoing[nodeName]; ok {
					return outgoing
				}
				return nil
			}

			// Evaluate conditional function
			targets := branch.Condition(ctx, view)
			return targets
		}
	}

	// No conditional edges, use regular outgoing edges
	if outgoing, ok := compiled.topology.outgoing[nodeName]; ok {
		return outgoing
	}

	return next
}
