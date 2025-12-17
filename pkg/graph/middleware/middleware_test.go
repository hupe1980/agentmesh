package middleware_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	graphmw "github.com/hupe1980/agentmesh/pkg/graph/middleware"
	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeNameFromScope(t *testing.T) {
	scope := testutil.NewTestScopeFromMap(nil).WithNodeName("myNode")
	assert.Equal(t, "myNode", scope.NodeName())
}

func TestChain(t *testing.T) {
	var order []int

	mw1 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			order = append(order, 1)
			cmd, err := next(ctx, scope)
			order = append(order, 10)
			return cmd, err
		}
	}

	mw2 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			order = append(order, 2)
			cmd, err := next(ctx, scope)
			order = append(order, 20)
			return cmd, err
		}
	}

	mw3 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			order = append(order, 3)
			cmd, err := next(ctx, scope)
			order = append(order, 30)
			return cmd, err
		}
	}

	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		order = append(order, 100)
		return graph.To(graph.END)
	}

	chained := graph.ChainNodeMiddleware(mw1, mw2, mw3)(inner)
	_, err := chained(context.Background(), nil)
	require.NoError(t, err)

	// Order should be: 1 -> 2 -> 3 -> 100 -> 30 -> 20 -> 10
	assert.Equal(t, []int{1, 2, 3, 100, 30, 20, 10}, order)
}

func TestLoggingMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mw := graphmw.LoggingMiddleware(logger)

	called := false
	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		called = true
		return graph.To(graph.END)
	}

	wrapped := mw(inner)
	scope := testutil.NewTestScopeFromMap(nil).WithNodeName("testNode")
	_, err := wrapped(context.Background(), scope)

	require.NoError(t, err)
	assert.True(t, called)
}

func TestLoggingMiddlewareWithError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mw := graphmw.LoggingMiddleware(logger)

	expectedErr := errors.New("test error")
	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return nil, expectedErr
	}

	wrapped := mw(inner)
	scope := testutil.NewTestScopeFromMap(nil).WithNodeName("errorNode")
	_, err := wrapped(context.Background(), scope)

	assert.ErrorIs(t, err, expectedErr)
}

func TestTimingMiddleware(t *testing.T) {
	var recordedNode string
	var recordedDuration time.Duration

	mw := graphmw.TimingMiddleware(func(nodeName string, d time.Duration) {
		recordedNode = nodeName
		recordedDuration = d
	})

	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		time.Sleep(10 * time.Millisecond)
		return graph.To(graph.END)
	}

	wrapped := mw(inner)
	scope := testutil.NewTestScopeFromMap(nil).WithNodeName("timedNode")
	_, err := wrapped(context.Background(), scope)

	require.NoError(t, err)
	assert.Equal(t, "timedNode", recordedNode)
	assert.GreaterOrEqual(t, recordedDuration, 10*time.Millisecond)
}

func TestTimingMiddlewareNilCallback(t *testing.T) {
	mw := graphmw.TimingMiddleware(nil)

	called := false
	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		called = true
		return graph.To(graph.END)
	}

	wrapped := mw(inner)
	scope := testutil.NewTestScopeFromMap(nil)
	_, err := wrapped(context.Background(), scope)

	require.NoError(t, err)
	assert.True(t, called)
}

func TestRecoveryMiddleware(t *testing.T) {
	var recoveredNode string
	var recoveredValue any

	mw := graphmw.RecoveryMiddleware(func(nodeName string, recovered any) {
		recoveredNode = nodeName
		recoveredValue = recovered
	})

	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		panic("test panic")
	}

	wrapped := mw(inner)
	scope := testutil.NewTestScopeFromMap(nil).WithNodeName("panicNode")
	_, err := wrapped(context.Background(), scope)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "panic in node panicNode")
	assert.Equal(t, "panicNode", recoveredNode)
	assert.Equal(t, "test panic", recoveredValue)
}

func TestRecoveryMiddlewareWithErrorPanic(t *testing.T) {
	panicErr := errors.New("panic error")

	mw := graphmw.RecoveryMiddleware(nil)

	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		panic(panicErr)
	}

	wrapped := mw(inner)
	scope := testutil.NewTestScopeFromMap(nil)
	_, err := wrapped(context.Background(), scope)

	assert.ErrorIs(t, err, panicErr)
}

func TestRecoveryMiddlewareNoPanic(t *testing.T) {
	mw := graphmw.RecoveryMiddleware(func(nodeName string, recovered any) {
		t.Fatal("should not be called")
	})

	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}

	wrapped := mw(inner)
	scope := testutil.NewTestScopeFromMap(nil)
	_, err := wrapped(context.Background(), scope)

	require.NoError(t, err)
}

func TestConditionalMiddleware(t *testing.T) {
	var innerCalled int32

	mw := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			atomic.AddInt32(&innerCalled, 1)
			return next(ctx, scope)
		}
	}

	condition := func(scope graph.Scope) bool {
		return scope.NodeName() == "apply"
	}

	conditionalMW := graphmw.ConditionalMiddleware(condition, mw)

	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}

	wrapped := conditionalMW(inner)

	// When condition is true
	scopeApply := testutil.NewTestScopeFromMap(nil).WithNodeName("apply")
	_, err := wrapped(context.Background(), scopeApply)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&innerCalled))

	// When condition is false
	scopeSkip := testutil.NewTestScopeFromMap(nil).WithNodeName("skip")
	_, err = wrapped(context.Background(), scopeSkip)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&innerCalled)) // Not incremented
}

func TestNodeNameMiddleware(t *testing.T) {
	var appliedNodes []string

	mw := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			appliedNodes = append(appliedNodes, scope.NodeName())
			return next(ctx, scope)
		}
	}

	nodeMW := graphmw.NodeNameMiddleware([]string{"nodeA", "nodeB"}, mw)

	inner := func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}

	wrapped := nodeMW(inner)

	// Apply to nodeA
	scopeA := testutil.NewTestScopeFromMap(nil).WithNodeName("nodeA")
	_, _ = wrapped(context.Background(), scopeA)

	// Apply to nodeB
	scopeB := testutil.NewTestScopeFromMap(nil).WithNodeName("nodeB")
	_, _ = wrapped(context.Background(), scopeB)

	// Should not apply to nodeC
	scopeC := testutil.NewTestScopeFromMap(nil).WithNodeName("nodeC")
	_, _ = wrapped(context.Background(), scopeC)

	assert.Equal(t, []string{"nodeA", "nodeB"}, appliedNodes)
}

func TestChainedMiddlewareInGraph(t *testing.T) {
	var order []string

	mw1 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			order = append(order, "mw1-before")
			cmd, err := next(ctx, scope)
			order = append(order, "mw1-after")
			return cmd, err
		}
	}

	mw2 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			order = append(order, "mw2-before")
			cmd, err := next(ctx, scope)
			order = append(order, "mw2-after")
			return cmd, err
		}
	}

	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)
	g.Node("a", func(_ context.Context, _ graph.Scope) (*graph.Command, error) {
		order = append(order, "node-execute")
		return graph.To(graph.END)
	}, graph.END)
	g.Start("a")
	g.WithNodeMiddleware(graph.ChainNodeMiddleware(mw1, mw2))

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
