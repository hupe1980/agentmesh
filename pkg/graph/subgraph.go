package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// InputMapper maps parent state to subgraph input.
// This provides type-safe data transfer from parent graph to subgraph.
//
// Example:
//
//	mapper := func(ctx context.Context, view state.ReadView) (MyInput, error) {
//	    return MyInput{
//	        UserID: state.GetFromView(view, userIDKey),
//	        Data:   state.GetFromView(view, dataKey),
//	    }, nil
//	}
type InputMapper[I any] func(ctx context.Context, view state.ReadView) (I, error)

// OutputMapper maps subgraph output to parent state updates.
// This provides type-safe data transfer from subgraph back to parent graph.
//
// Example:
//
//	mapper := func(ctx context.Context, output MyOutput) (state.Updates, error) {
//	    return state.Updates{
//	        resultKey.Name(): output.Result,
//	        statusKey.Name(): "completed",
//	    }, nil
//	}
type OutputMapper[O any] func(ctx context.Context, output O) (state.Updates, error)

// SubgraphNode is a node that executes a compiled subgraph.
// It provides type-safe input/output mapping between parent and subgraph state.
//
// The subgraph runs in isolation with its own state management, but can exchange
// data with the parent graph through typed mappers.
//
// Example:
//
//	// Create validation subgraph
//	validationGraph, _ := buildValidationGraph()
//	validationCompiled, _ := Compile(validationGraph, executor)
//
//	// Create subgraph node with type-safe mappers
//	subgraphNode := graph.NewSubgraphNode(
//	    "validation",
//	    validationCompiled,
//	    func(ctx context.Context, view state.ReadView) (ValidationInput, error) {
//	        return ValidationInput{
//	            Data: state.GetFromView(view, dataKey),
//	        }, nil
//	    },
//	    func(ctx context.Context, output ValidationOutput) (state.Updates, error) {
//	        return state.Updates{
//	            validKey.Name(): output.Valid,
//	            errorsKey.Name(): output.Errors,
//	        }, nil
//	    },
//	    []string{"process", graph.EndNode},
//	)
//
//	// Add to parent graph
//	parentGraph.AddNode(subgraphNode)
type SubgraphNode[I, O any] struct {
	name         string
	compiled     *Compiled[I, O]
	inputMapper  InputMapper[I]
	outputMapper OutputMapper[O]
	targets      []string
	retryPolicy  *RetryPolicy
	version      string
	metadata     map[string]string
}

// SubgraphOption configures a subgraph node.
type SubgraphOption func(*subgraphConfig)

type subgraphConfig struct {
	retryPolicy *RetryPolicy
	version     string
	metadata    map[string]string
}

// WithSubgraphRetry sets retry policy for subgraph execution.
func WithSubgraphRetry(policy *RetryPolicy) SubgraphOption {
	return func(c *subgraphConfig) {
		c.retryPolicy = policy
	}
}

// WithSubgraphVersion sets the version identifier for the subgraph.
// Useful for tracking which version of a subgraph is being used.
func WithSubgraphVersion(version string) SubgraphOption {
	return func(c *subgraphConfig) {
		c.version = version
	}
}

// WithSubgraphMetadata adds metadata to the subgraph node.
// Useful for documentation, debugging, and graph introspection.
func WithSubgraphMetadata(key, value string) SubgraphOption {
	return func(c *subgraphConfig) {
		if c.metadata == nil {
			c.metadata = make(map[string]string)
		}
		c.metadata[key] = value
	}
}

// NewSubgraphNode creates a new subgraph node with type-safe input/output mappers.
//
// Parameters:
//   - name: unique node identifier
//   - compiled: compiled subgraph to execute
//   - inputMapper: maps parent state to subgraph input
//   - outputMapper: maps subgraph output to parent state updates
//   - targets: possible routing targets after subgraph completes
//   - opts: optional configuration (retry, version, metadata)
//
// The subgraph executes with full isolation - it has its own state manager
// and cannot directly access parent state. Data flows through the mappers.
func NewSubgraphNode[I, O any](
	name string,
	compiled *Compiled[I, O],
	inputMapper InputMapper[I],
	outputMapper OutputMapper[O],
	targets []string,
	opts ...SubgraphOption,
) *SubgraphNode[I, O] {
	cfg := &subgraphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return &SubgraphNode[I, O]{
		name:         name,
		compiled:     compiled,
		inputMapper:  inputMapper,
		outputMapper: outputMapper,
		targets:      targets,
		retryPolicy:  cfg.retryPolicy,
		version:      cfg.version,
		metadata:     cfg.metadata,
	}
}

// Name returns the node's name.
func (n *SubgraphNode[I, O]) Name() string {
	return n.name
}

// Execute runs the subgraph with mapped input and returns mapped output.
//
// Execution flow:
//  1. Map parent state → subgraph input (via inputMapper)
//  2. Execute subgraph with input
//  3. Collect final output from subgraph
//  4. Map subgraph output → parent state updates (via outputMapper)
//  5. Return targets and updates to parent graph
//
// The subgraph runs in complete isolation - it cannot access parent state
// except through the input mapper.
func (n *SubgraphNode[I, O]) Execute(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
	// Map parent state to subgraph input
	input, err := n.inputMapper(ctx, view)
	if err != nil {
		return nil, nil, fmt.Errorf("subgraph %q input mapping failed: %w", n.name, err)
	}

	// Execute subgraph and collect final output
	var finalOutput O
	var lastErr error

	for output, err := range n.compiled.Run(ctx, input) {
		if err != nil {
			lastErr = err
			break
		}
		finalOutput = output
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("subgraph %q execution failed: %w", n.name, lastErr)
	}

	// Map subgraph output to parent state updates
	updates, err := n.outputMapper(ctx, finalOutput)
	if err != nil {
		return nil, nil, fmt.Errorf("subgraph %q output mapping failed: %w", n.name, err)
	}

	return n.targets, updates, nil
}

// Targets returns the declared routing targets.
func (n *SubgraphNode[I, O]) Targets() []string {
	return n.targets
}

// RetryPolicy returns the retry policy if set.
func (n *SubgraphNode[I, O]) RetryPolicy() *RetryPolicy {
	return n.retryPolicy
}

// Version returns the subgraph version.
func (n *SubgraphNode[I, O]) Version() string {
	return n.version
}

// Metadata returns the subgraph metadata.
func (n *SubgraphNode[I, O]) Metadata() map[string]string {
	if n.metadata == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(n.metadata))
	for k, v := range n.metadata {
		result[k] = v
	}
	return result
}

// Compiled returns the underlying compiled subgraph.
// Useful for introspection and debugging.
func (n *SubgraphNode[I, O]) Compiled() *Compiled[I, O] {
	return n.compiled
}

// PassthroughInputMapper is a helper that passes input directly without transformation.
// Use when subgraph input type matches parent state structure.
func PassthroughInputMapper[I any](input I) InputMapper[I] {
	return func(ctx context.Context, view state.ReadView) (I, error) {
		return input, nil
	}
}

// PassthroughOutputMapper is a helper that passes output directly as updates.
// Use when subgraph output is already in Updates format.
func PassthroughOutputMapper() OutputMapper[state.Updates] {
	return func(ctx context.Context, output state.Updates) (state.Updates, error) {
		return output, nil
	}
}

// SimpleInputMapper creates an input mapper from a single key.
// Useful for simple subgraphs that take a single input value.
//
// Example:
//
//	mapper := graph.SimpleInputMapper(dataKey)
//	// Extracts dataKey from parent state and passes as input
func SimpleInputMapper[T any](key state.Key[T]) InputMapper[T] {
	return func(ctx context.Context, view state.ReadView) (T, error) {
		return state.GetFromView(view, key), nil
	}
}

// SimpleOutputMapper creates an output mapper that stores output in a single key.
// Useful for simple subgraphs that produce a single output value.
//
// Example:
//
//	mapper := graph.SimpleOutputMapper(resultKey)
//	// Stores subgraph output in resultKey
func SimpleOutputMapper[T any](key state.Key[T]) OutputMapper[T] {
	return func(ctx context.Context, output T) (state.Updates, error) {
		return state.Updates{
			key.Name(): output,
		}, nil
	}
}
