package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/pregel"
)

func TestMaxIterations(t *testing.T) {
	t.Run("terminates cyclic graph at max iterations", func(t *testing.T) {
		state := NewStateManager(0)
		g := NewGraph(state)

		// Create a self-loop that increments counter
		if err := g.AddNode(&Node{
			Name: "looper",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				counter, _ := s.Get("counter").(int)
				return &NodeResult{
					Updates: map[string]any{"counter": counter + 1},
				}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		// Self-loop: looper -> looper (infinite without max iterations)
		g.AddEdge(StartNode, "looper")
		g.AddEdge("looper", "looper")

		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Should terminate after 5 iterations
		_, err = Last(compiled.Run(context.Background(), nil, WithMaxIterations(5)))
		if err == nil {
			t.Fatal("expected ErrMaxIterationsExceeded, got nil")
		}
		if !errors.Is(err, ErrMaxIterationsExceeded) && !errors.Is(err, pregel.ErrMaxIterationsExceeded) {
			t.Fatalf("expected ErrMaxIterationsExceeded, got %v", err)
		}

		// Counter should have incremented 5 times
		counter, ok := compiled.State().Get("counter").(int)
		if !ok {
			t.Fatal("counter not found")
		}
		if counter != 5 {
			t.Errorf("expected counter=5, got %d", counter)
		}
	})

	t.Run("unlimited iterations when not specified", func(t *testing.T) {
		state := NewStateManager(0); state.Set("count", 0)
		g := NewGraph(state)

		if err := g.AddNode(&Node{
			Name: "simple",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
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
		state := NewStateManager(0)
		g := NewGraph(state)

		if err := g.AddNode(&Node{
			Name: "simple",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}

		g.AddEdge(StartNode, "simple")

		compiled, err := g.Compile()
		if err != nil {
			t.Fatal(err)
		}

		// Max iterations is 100 but should complete in 1 iteration
		_, err = Last(compiled.Run(context.Background(), nil, WithMaxIterations(100)))
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Should have completed in 1 superstep
		if compiled.CurrentSuperstep() > 2 {
			t.Errorf("expected superstep <= 2, got %d", compiled.CurrentSuperstep())
		}
	})
}
