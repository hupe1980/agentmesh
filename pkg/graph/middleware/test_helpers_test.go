package middleware

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// mockExecutor is a test helper that implements graph.Executor
type mockExecutor[I, O any] struct {
	runFunc func(ctx context.Context, compiled *graph.Compiled[I, O], input I, opts ...graph.RunOption) iter.Seq2[O, error]
}

func (m *mockExecutor[I, O]) Run(ctx context.Context, compiled *graph.Compiled[I, O], input I, opts ...graph.RunOption) iter.Seq2[O, error] {
	return m.runFunc(ctx, compiled, input, opts...)
}

// captureEvents is a test helper that captures all events published to the event bus
type captureEvents struct {
	events []graph.Event
}

func (c *captureEvents) HandleEvent(ctx context.Context, event graph.Event) error {
	c.events = append(c.events, event)
	return nil
}
