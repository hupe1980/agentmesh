package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/pregel"
	stateif "github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/require"
)

func TestMaxIterations(t *testing.T) {
	t.Run("simple execution works", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		executed := false
		if err := g.AddNode(&Node{
			Name: "simple",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				executed = true
				return &NodeResult{}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "simple")

		// Try with custom executor
		executor := NewPregelExecutor(WithPregelMaxIterations(10))
		g, err = g.WithExecutor(executor)
		require.NoError(t, err)

		compiled, err := g.Compile()
		require.NoError(t, err)

		_, err = Last(compiled.Run(context.Background(), nil))
		require.NoError(t, err)
		require.True(t, executed, "Node should have executed")
	})

	t.Run("terminates cyclic graph at max iterations", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		state.Set("counter", 0) // Initialize counter
		g, err := NewGraph(state)
		require.NoError(t, err)

		// Create a node that loops via conditional edge
		if err := g.AddNode(&Node{
			Name: "looper",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				counter, _ := s.Get("counter").(int)
				return &NodeResult{
					Updates: map[string]any{"counter": counter + 1},
				}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		// Start -> looper, then looper conditionally goes back to itself
		g.AddEdge(StartNode, "looper")
		g.AddConditionalEdges("looper", func(ctx context.Context, s stateif.Reader) []string {
			// Loop back to self to create infinite loop (would run forever without max iterations)
			return []string{"looper"}
		}, []string{"looper"})

		// Configure executor with max iterations to prevent infinite loop
		executor := NewPregelExecutor(WithPregelMaxIterations(5))
		g, err = g.WithExecutor(executor)
		require.NoError(t, err)

		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Should terminate after 5 iterations
		ctx := context.Background()
		var lastErr error
		for event := range compiled.Run(ctx, nil) {
			if event.Err != nil {
				lastErr = event.Err
			}
		}

		// Should have hit max iterations error
		require.Error(t, lastErr, "expected error for max iterations exceeded")
		require.True(t,
			errors.Is(lastErr, ErrMaxIterationsExceeded) || errors.Is(lastErr, pregel.ErrMaxIterationsExceeded),
			"expected ErrMaxIterationsExceeded, got %v", lastErr)

		// Counter should have incremented 5 times (one per superstep)
		counter, ok := compiled.State().Get("counter").(int)
		require.True(t, ok, "counter should exist")
		require.Equal(t, 5, counter, "counter should be 5 after 5 iterations")
	})

	t.Run("unlimited iterations when not specified", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		state.Set("count", 0)
		g, err := NewGraph(state)
		require.NoError(t, err)

		if err := g.AddNode(&Node{
			Name: "simple",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				return &NodeResult{
					Updates: map[string]any{"count": 42},
				}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "simple")

		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Should complete naturally (no max iterations specified)
		_, err = Last(compiled.Run(context.Background(), nil))
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		count, ok := compiled.State().Get("count").(int)
		if !ok {
			t.Fatal("count not found or wrong type")
		}
		if count != 42 {
			t.Errorf("expected count=42, got %d", count)
		}
	})

	t.Run("terminates before max iterations if quiesced", func(t *testing.T) {
		state, err := NewStateManager(0)
		require.NoError(t, err)
		g, err := NewGraph(state)
		require.NoError(t, err)

		if err := g.AddNode(&Node{
			Name: "simple",
			RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "simple")

		// Configure PregelExecutor with max iterations
		executor := NewPregelExecutor(WithPregelMaxIterations(100))
		if _, err := g.WithExecutor(executor); err != nil {
			t.Fatal(err)
		}

		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Max iterations is 100 but should complete in 1 iteration
		_, err = Last(compiled.Run(context.Background(), nil))
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Should have completed in 1 superstep
		if compiled.CurrentSuperstep() > 2 {
			t.Errorf("expected superstep <= 2, got %d", compiled.CurrentSuperstep())
		}
	})
}
