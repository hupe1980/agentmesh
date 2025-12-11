package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// ====================
// Graph Builder Tests
// ====================

func TestGraphBuilder(t *testing.T) {
	counterKey := graph.NewKey("counter", 0)

	g := graph.New[any, any](counterKey)
	g.Node("process", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		return graph.Set(counterKey, counter+1).End()
	}, graph.END)
	g.Start("process")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Just verify the graph runs without error
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}
}

func TestGraphLinearFlow(t *testing.T) {
	counterKey := graph.NewKey("counter", 0)
	executed := make([]string, 0)

	g := graph.New[any, any](counterKey)
	g.Node("step1", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		executed = append(executed, "step1")
		return graph.Set(counterKey, 1).To("step2")
	}, "step2")
	g.Node("step2", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		executed = append(executed, "step2")
		counter := graph.Get(scope, counterKey)
		return graph.Set(counterKey, counter+10).To("step3")
	}, "step3")
	g.Node("step3", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		executed = append(executed, "step3")
		counter := graph.Get(scope, counterKey)
		if counter != 11 {
			t.Errorf("expected counter=11, got %d", counter)
		}
		return graph.Set(counterKey, counter+100).End()
	}, graph.END)
	g.Start("step1")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Verify all steps executed
	if len(executed) != 3 {
		t.Errorf("expected 3 steps, got %d: %v", len(executed), executed)
	}
}

func TestGraphConditionalRouting(t *testing.T) {
	routeKey := graph.NewKey("route", "")
	resultKey := graph.NewKey("result", "")
	var finalResult string

	g := graph.New[string, any](routeKey, resultKey)
	g.Node("router", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		route := graph.Get(scope, routeKey)
		if route == "left" {
			return graph.To("left")
		}
		return graph.To("right")
	}, "left", "right")
	g.Node("left", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		finalResult = "went_left"
		return graph.Set(resultKey, "went_left").End()
	}, graph.END)
	g.Node("right", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		finalResult = "went_right"
		return graph.Set(resultKey, "went_right").End()
	}, graph.END)
	g.Start("router")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// The input "left" is set into the routeKey (first key passed to New)
	// So the router should route to "left"
	for _, err := range compiled.Run(context.Background(), "left") {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Input "left" is set into routeKey, so it goes left
	if finalResult != "went_left" {
		t.Errorf("expected went_left, got %v", finalResult)
	}
}

func TestGraphLoop(t *testing.T) {
	counterKey := graph.NewKey("counter", 0)
	maxIterations := 5
	iterations := 0

	g := graph.New[any, any](counterKey)
	g.Node("increment", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		counter++
		iterations++
		if counter >= maxIterations {
			return graph.Set(counterKey, counter).End()
		}
		return graph.Set(counterKey, counter).To("increment")
	}, "increment", graph.END)
	g.Start("increment")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if iterations != maxIterations {
		t.Errorf("expected %d iterations, got %d", maxIterations, iterations)
	}
}

func TestGraphErrorHandling(t *testing.T) {
	expectedErr := errors.New("node error")

	g := graph.New[any, any]()
	g.Node("failing", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		return graph.Fail(expectedErr)
	}, graph.END)
	g.Start("failing")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	var gotErr error
	for _, err := range compiled.Run(context.Background(), nil) {
		gotErr = err
	}

	if gotErr == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(gotErr, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, gotErr)
	}
}

func TestGraphMessageList(t *testing.T) {
	messagesKey := graph.NewListKey[string]("messages")
	var capturedMessages []string

	g := graph.New[any, any](messagesKey)
	g.Node("add_messages", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		return graph.Append(messagesKey, "hello", "world").To("check")
	}, "check")
	g.Node("check", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		capturedMessages = graph.GetList(scope, messagesKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("add_messages")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if len(capturedMessages) != 2 || capturedMessages[0] != "hello" || capturedMessages[1] != "world" {
		t.Errorf("expected [hello, world], got %v", capturedMessages)
	}
}

// ====================
// Validation Tests
// ====================

func TestGraphNoEntryPoint(t *testing.T) {
	g := graph.New[any, any]()
	g.Node("a", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	// No Start() call

	_, err := g.Build()
	if err == nil {
		t.Fatal("expected error for missing entry point")
	}
}

func TestGraphDuplicateNodeOverwrites(t *testing.T) {
	resultKey := graph.NewKey("result", "")
	var capturedResult string

	g := graph.New[any, any](resultKey)
	g.Node("a", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		capturedResult = "first"
		return graph.Set(resultKey, "first").End()
	}, graph.END)
	// Second definition overwrites first
	g.Node("a", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		capturedResult = "second"
		return graph.Set(resultKey, "second").End()
	}, graph.END)
	g.Start("a")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Should use the second (overwritten) definition
	if capturedResult != "second" {
		t.Errorf("expected second, got %v", capturedResult)
	}
}

func TestWithInitialValue(t *testing.T) {
	sessionKey := graph.NewKey("session_id", "default-session")
	var capturedSession string

	g := graph.New[any, any](sessionKey)
	g.Node("process", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		capturedSession = graph.Get(scope, sessionKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("process")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	t.Run("uses default when no initial value", func(t *testing.T) {
		capturedSession = ""
		for _, err := range compiled.Run(context.Background(), nil) {
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
		}
		if capturedSession != "default-session" {
			t.Errorf("expected default-session, got %v", capturedSession)
		}
	})

	t.Run("uses provided initial value", func(t *testing.T) {
		capturedSession = ""
		for _, err := range compiled.Run(context.Background(), nil,
			graph.WithInitialValue(sessionKey, "custom-session"),
		) {
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
		}
		if capturedSession != "custom-session" {
			t.Errorf("expected custom-session, got %v", capturedSession)
		}
	})
}
