package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/pregel"
)

type cancelState struct{}
type cancelMessage struct{}

// singleVertexGraph executes a single vertex that immediately blocks on mailbox receive
// so we can observe cancellation behavior through the runtime.
type singleVertexGraph struct {
	vertex *singleVertex
}

type singleVertex struct {
	name string
}

func newSingleVertexGraph(name string) *singleVertexGraph {
	return &singleVertexGraph{vertex: &singleVertex{name: name}}
}

func (g *singleVertexGraph) RootVertices() []string { return []string{g.vertex.name} }

func (g *singleVertexGraph) Outgoing(vertex string) []string { return nil }

func (g *singleVertexGraph) VertexByName(name string) pregel.Vertex[cancelState, cancelMessage] {
	if g.vertex.name == name {
		return g.vertex
	}
	return nil
}

func (g *singleVertexGraph) State() cancelState { return cancelState{} }

func (v *singleVertex) Name() string { return v.name }

func (v *singleVertex) Run(ctx context.Context, vertex pregel.VertexContext[cancelState, cancelMessage], incoming []pregel.Message[cancelMessage]) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// blockingMessageBus never returns from Receive until the context is cancelled.
type blockingMessageBus struct {
	started chan struct{}
}

func newBlockingMessageBus() *blockingMessageBus {
	return &blockingMessageBus{started: make(chan struct{}, 1)}
}

func (b *blockingMessageBus) Send(ctx context.Context, messages []pregel.Message[cancelMessage]) error {
	return nil
}

func (b *blockingMessageBus) Receive(ctx context.Context, vertex string) ([]pregel.Message[cancelMessage], error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingMessageBus) Clear(ctx context.Context, vertex string) error { return nil }

func (b *blockingMessageBus) Close() error { return nil }

// chaosMessageBus simulates a slow backend by sleeping before returning unless
// the context is cancelled first.
type chaosMessageBus struct {
	delay   time.Duration
	started chan struct{}
}

func newChaosMessageBus(delay time.Duration) *chaosMessageBus {
	return &chaosMessageBus{delay: delay, started: make(chan struct{}, 1)}
}

func (c *chaosMessageBus) Send(ctx context.Context, messages []pregel.Message[cancelMessage]) error {
	return nil
}

func (c *chaosMessageBus) Receive(ctx context.Context, vertex string) ([]pregel.Message[cancelMessage], error) {
	select {
	case c.started <- struct{}{}:
	default:
	}

	timer := time.NewTimer(c.delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, nil
	}
}

func (c *chaosMessageBus) Clear(ctx context.Context, vertex string) error { return nil }

func (c *chaosMessageBus) Close() error { return nil }

func drainRuntime(ctx context.Context, rt *pregel.Runtime[cancelState, cancelMessage]) error {
	for _, err := range rt.Run(ctx) {
		if err != nil {
			return err
		}
	}
	return nil
}

func TestRuntimeReceiveCancellation(t *testing.T) {
	t.Parallel()

	graph := newSingleVertexGraph("root")
	bus := newBlockingMessageBus()
	rt := pregel.MustNewRuntime[cancelState, cancelMessage](graph, nil,
		pregel.WithMessageBus[cancelState, cancelMessage](bus),
		pregel.WithMaxWorkers[cancelState, cancelMessage](1),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- drainRuntime(ctx, rt)
	}()

	select {
	case <-bus.started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker never attempted Receive")
	}

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected context cancellation error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not exit after cancellation")
	}
}

func TestRuntimeChaosLatencyBounded(t *testing.T) {
	t.Parallel()

	graph := newSingleVertexGraph("root")
	delay := 3 * time.Second
	bus := newChaosMessageBus(delay)

	rt := pregel.MustNewRuntime[cancelState, cancelMessage](graph, nil,
		pregel.WithMessageBus[cancelState, cancelMessage](bus),
		pregel.WithMaxWorkers[cancelState, cancelMessage](1),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- drainRuntime(ctx, rt)
	}()

	select {
	case <-bus.started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker never attempted Receive")
	}

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled error, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("runtime took too long to exit after cancellation")
	}
}
