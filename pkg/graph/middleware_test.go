package graph_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithNodeName(t *testing.T) {
	ctx := context.Background()

	// Initially no node name
	assert.Equal(t, "", graph.NodeNameFromContext(ctx))

	// Attach node name
	ctx = graph.WithNodeName(ctx, "myNode")
	assert.Equal(t, "myNode", graph.NodeNameFromContext(ctx))
}

func TestChain(t *testing.T) {
	var order []int

	mw1 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, view graph.View) (*graph.Command, error) {
			order = append(order, 1)
			cmd, err := next(ctx, view)
			order = append(order, 10)
			return cmd, err
		}
	}

	mw2 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, view graph.View) (*graph.Command, error) {
			order = append(order, 2)
			cmd, err := next(ctx, view)
			order = append(order, 20)
			return cmd, err
		}
	}

	mw3 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, view graph.View) (*graph.Command, error) {
			order = append(order, 3)
			cmd, err := next(ctx, view)
			order = append(order, 30)
			return cmd, err
		}
	}

	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		order = append(order, 100)
		return graph.To(graph.END)
	}

	chained := graph.Chain(mw1, mw2, mw3)(inner)
	_, err := chained(context.Background(), nil)
	require.NoError(t, err)

	// Order should be: 1 -> 2 -> 3 -> 100 -> 30 -> 20 -> 10
	assert.Equal(t, []int{1, 2, 3, 100, 30, 20, 10}, order)
}

func TestLoggingMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mw := graph.LoggingMiddleware(logger)

	called := false
	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		called = true
		return graph.To(graph.END)
	}

	wrapped := mw(inner)
	ctx := graph.WithNodeName(context.Background(), "testNode")
	_, err := wrapped(ctx, nil)

	require.NoError(t, err)
	assert.True(t, called)
}

func TestLoggingMiddlewareWithError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mw := graph.LoggingMiddleware(logger)

	expectedErr := errors.New("test error")
	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return nil, expectedErr
	}

	wrapped := mw(inner)
	ctx := graph.WithNodeName(context.Background(), "errorNode")
	_, err := wrapped(ctx, nil)

	assert.ErrorIs(t, err, expectedErr)
}

func TestTimingMiddleware(t *testing.T) {
	var recordedNode string
	var recordedDuration time.Duration

	mw := graph.TimingMiddleware(func(nodeName string, d time.Duration) {
		recordedNode = nodeName
		recordedDuration = d
	})

	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		time.Sleep(10 * time.Millisecond)
		return graph.To(graph.END)
	}

	wrapped := mw(inner)
	ctx := graph.WithNodeName(context.Background(), "timedNode")
	_, err := wrapped(ctx, nil)

	require.NoError(t, err)
	assert.Equal(t, "timedNode", recordedNode)
	assert.GreaterOrEqual(t, recordedDuration, 10*time.Millisecond)
}

func TestTimingMiddlewareNilCallback(t *testing.T) {
	mw := graph.TimingMiddleware(nil)

	called := false
	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		called = true
		return graph.To(graph.END)
	}

	wrapped := mw(inner)
	_, err := wrapped(context.Background(), nil)

	require.NoError(t, err)
	assert.True(t, called)
}

func TestRecoveryMiddleware(t *testing.T) {
	var recoveredNode string
	var recoveredValue any

	mw := graph.RecoveryMiddleware(func(nodeName string, recovered any) {
		recoveredNode = nodeName
		recoveredValue = recovered
	})

	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		panic("test panic")
	}

	wrapped := mw(inner)
	ctx := graph.WithNodeName(context.Background(), "panicNode")
	_, err := wrapped(ctx, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "panic in node panicNode")
	assert.Equal(t, "panicNode", recoveredNode)
	assert.Equal(t, "test panic", recoveredValue)
}

func TestRecoveryMiddlewareWithErrorPanic(t *testing.T) {
	panicErr := errors.New("panic error")

	mw := graph.RecoveryMiddleware(nil)

	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		panic(panicErr)
	}

	wrapped := mw(inner)
	_, err := wrapped(context.Background(), nil)

	assert.ErrorIs(t, err, panicErr)
}

func TestRecoveryMiddlewareNoPanic(t *testing.T) {
	mw := graph.RecoveryMiddleware(func(nodeName string, recovered any) {
		t.Fatal("should not be called")
	})

	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.To(graph.END)
	}

	wrapped := mw(inner)
	_, err := wrapped(context.Background(), nil)

	require.NoError(t, err)
}

func TestConditionalMiddleware(t *testing.T) {
	var innerCalled int32

	mw := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, view graph.View) (*graph.Command, error) {
			atomic.AddInt32(&innerCalled, 1)
			return next(ctx, view)
		}
	}

	condition := func(ctx context.Context) bool {
		return graph.NodeNameFromContext(ctx) == "apply"
	}

	conditionalMW := graph.ConditionalMiddleware(condition, mw)

	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.To(graph.END)
	}

	wrapped := conditionalMW(inner)

	// When condition is true
	ctx := graph.WithNodeName(context.Background(), "apply")
	_, err := wrapped(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&innerCalled))

	// When condition is false
	ctx = graph.WithNodeName(context.Background(), "skip")
	_, err = wrapped(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&innerCalled)) // Not incremented
}

func TestNodeMiddleware(t *testing.T) {
	var appliedNodes []string

	mw := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, view graph.View) (*graph.Command, error) {
			appliedNodes = append(appliedNodes, graph.NodeNameFromContext(ctx))
			return next(ctx, view)
		}
	}

	nodeMW := graph.NodeMiddleware([]string{"nodeA", "nodeB"}, mw)

	inner := func(_ context.Context, _ graph.View) (*graph.Command, error) {
		return graph.To(graph.END)
	}

	wrapped := nodeMW(inner)

	// Apply to nodeA
	ctx := graph.WithNodeName(context.Background(), "nodeA")
	_, _ = wrapped(ctx, nil)

	// Apply to nodeB
	ctx = graph.WithNodeName(context.Background(), "nodeB")
	_, _ = wrapped(ctx, nil)

	// Should not apply to nodeC
	ctx = graph.WithNodeName(context.Background(), "nodeC")
	_, _ = wrapped(ctx, nil)

	assert.Equal(t, []string{"nodeA", "nodeB"}, appliedNodes)
}

func TestChainedMiddlewareInGraph(t *testing.T) {
	var order []string

	mw1 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, view graph.View) (*graph.Command, error) {
			order = append(order, "mw1-before")
			cmd, err := next(ctx, view)
			order = append(order, "mw1-after")
			return cmd, err
		}
	}

	mw2 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, view graph.View) (*graph.Command, error) {
			order = append(order, "mw2-before")
			cmd, err := next(ctx, view)
			order = append(order, "mw2-after")
			return cmd, err
		}
	}

	counterKey := graph.NewKey("counter", 0)

	g := graph.New[any, any](counterKey)
	g.Node("a", func(_ context.Context, _ graph.View) (*graph.Command, error) {
		order = append(order, "node-execute")
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a")
	g.WithMiddleware(graph.Chain(mw1, mw2))

	compiled, err := g.Build()
	require.NoError(t, err)

	for range compiled.Run(context.Background(), nil) {
	}

	assert.Equal(t, []string{
		"mw1-before",
		"mw2-before",
		"node-execute",
		"mw2-after",
		"mw1-after",
	}, order)
}
