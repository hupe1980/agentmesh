package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper: create a minimal compiled graph for testing
func createTestCompiledGraph(t *testing.T, nodes map[string]Node) *Compiled[[]message.Message, message.Message] {
	t.Helper()

	builder := state.NewManagerBuilder()
	mgr := builder.Build()
	g, err := NewGraph(mgr)
	require.NoError(t, err)

	// Add nodes
	var firstNodeName string
	for name, node := range nodes {
		err := g.AddNode(node)
		require.NoError(t, err)
		if firstNodeName == "" {
			firstNodeName = name
		}
	}

	// Set entry point
	if firstNodeName != "" {
		g.SetEntryPoint(firstNodeName)
	}

	// Compile
	executor := NewMessagePregelExecutor()
	compiled, err := Compile[[]message.Message, message.Message](g, executor)
	require.NoError(t, err)

	return compiled
}

// TestCheckNodeExists tests the checkNodeExists method.
func TestCheckNodeExists(t *testing.T) {
	t.Run("returns node when exists", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		node := adapter.checkNodeExists()
		assert.NotNil(t, node)
		assert.Equal(t, "test", node.Name())
	})

	t.Run("returns nil when node doesn't exist", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "nonexistent",
			compiled: compiled,
		}

		node := adapter.checkNodeExists()
		assert.Nil(t, node)
	})
}

// TestCheckPauseState tests the checkPauseState method.
func TestCheckPauseState(t *testing.T) {
	t.Run("returns false when no executor", func(t *testing.T) {
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: nil,
		}

		assert.False(t, adapter.checkPauseState())
	})

	t.Run("returns false when no metrics", func(t *testing.T) {
		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: nil,
		}
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
		}

		assert.False(t, adapter.checkPauseState())
	})

	t.Run("returns true when node is paused", func(t *testing.T) {
		metrics := NewRuntimeMetrics()
		metrics.AddPaused("test")

		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: metrics,
		}
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
		}

		assert.True(t, adapter.checkPauseState())
	})

	t.Run("returns false when different node is paused", func(t *testing.T) {
		metrics := NewRuntimeMetrics()
		metrics.AddPaused("other")

		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: metrics,
		}
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
		}

		assert.False(t, adapter.checkPauseState())
	})
}

// TestCheckIsResuming tests the checkIsResuming method.
func TestCheckIsResuming(t *testing.T) {
	t.Run("returns false when no executor", func(t *testing.T) {
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: nil,
		}

		assert.False(t, adapter.checkIsResuming())
	})

	t.Run("returns true when node is resuming", func(t *testing.T) {
		metrics := NewRuntimeMetrics()
		// Simulate a resume by first pausing, then resuming
		metrics.AddPaused("test")
		metrics.ResumePaused("test")

		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: metrics,
		}
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
		}

		assert.True(t, adapter.checkIsResuming())
	})

	t.Run("returns false when not resuming", func(t *testing.T) {
		metrics := NewRuntimeMetrics()

		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: metrics,
		}
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
		}

		assert.False(t, adapter.checkIsResuming())
	})
}

// TestPrepareStateView tests the prepareStateView method.
func TestPrepareStateView(t *testing.T) {
	t.Run("returns superstep view when available", func(t *testing.T) {
		builder := state.NewManagerBuilder()
		mgr := builder.Build()

		ctx := context.Background()
		view, err := mgr.CreateReadView(ctx)
		require.NoError(t, err)

		graphAdapter := &pregelGraphAdapter[[]message.Message, message.Message]{
			currentSuperstepView: view,
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName:     "test",
			graphAdapter: graphAdapter,
		}

		result, err := adapter.prepareStateView(ctx)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("creates fallback view when superstep view is nil", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		graphAdapter := &pregelGraphAdapter[[]message.Message, message.Message]{
			currentSuperstepView: nil,
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName:     "test",
			compiled:     compiled,
			graphAdapter: graphAdapter,
		}

		ctx := context.Background()
		result, err := adapter.prepareStateView(ctx)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})
}

// TestClearResumingFlag tests the clearResumingFlag method.
func TestClearResumingFlag(t *testing.T) {
	t.Run("clears flag when resuming", func(t *testing.T) {
		metrics := NewRuntimeMetrics()
		metrics.AddPaused("test")
		metrics.ResumePaused("test")

		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: metrics,
		}
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
		}

		assert.True(t, adapter.checkIsResuming())
		adapter.clearResumingFlag(true)
		assert.False(t, adapter.checkIsResuming())
	})

	t.Run("does nothing when not resuming", func(t *testing.T) {
		metrics := NewRuntimeMetrics()

		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: metrics,
		}
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
		}

		// Should not panic
		adapter.clearResumingFlag(false)
	})

	t.Run("does nothing when no executor", func(t *testing.T) {
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: nil,
		}

		// Should not panic
		adapter.clearResumingFlag(true)
	})
}

// TestValidateRoutingTargets tests the validateRoutingTargets method.
func TestValidateRoutingTargets(t *testing.T) {
	adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
		nodeName: "test",
	}

	t.Run("returns nil for nil targets", func(t *testing.T) {
		err := adapter.validateRoutingTargets(nil)
		assert.NoError(t, err)
	})

	t.Run("returns error for empty targets", func(t *testing.T) {
		err := adapter.validateRoutingTargets([]string{})
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrRoutingTargets))
	})

	t.Run("returns nil for valid targets", func(t *testing.T) {
		err := adapter.validateRoutingTargets([]string{EndNode})
		assert.NoError(t, err)

		err = adapter.validateRoutingTargets([]string{"next", "other"})
		assert.NoError(t, err)
	})
}

// TestTrackNodeCompletion tests the trackNodeCompletion method.
func TestTrackNodeCompletion(t *testing.T) {
	t.Run("tracks completion with metrics", func(t *testing.T) {
		metrics := NewRuntimeMetrics()

		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: metrics,
		}
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
		}

		adapter.trackNodeCompletion()

		snapshot := metrics.Snapshot()
		assert.Contains(t, snapshot.CompletedNodes, "test")
	})

	t.Run("does nothing without executor", func(t *testing.T) {
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: nil,
		}

		// Should not panic
		adapter.trackNodeCompletion()
	})
}

// TestTrackNodeExecution tests the trackNodeExecution method.
func TestTrackNodeExecution(t *testing.T) {
	t.Run("tracks execution in graph adapter", func(t *testing.T) {
		graphAdapter := &pregelGraphAdapter[[]message.Message, message.Message]{}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName:     "test",
			graphAdapter: graphAdapter,
		}

		adapter.trackNodeExecution()

		graphAdapter.mu.RLock()
		assert.True(t, graphAdapter.executedNodes["test"])
		graphAdapter.mu.RUnlock()
	})

	t.Run("initializes map if nil", func(t *testing.T) {
		graphAdapter := &pregelGraphAdapter[[]message.Message, message.Message]{
			executedNodes: nil,
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName:     "test",
			graphAdapter: graphAdapter,
		}

		adapter.trackNodeExecution()

		graphAdapter.mu.RLock()
		assert.NotNil(t, graphAdapter.executedNodes)
		assert.True(t, graphAdapter.executedNodes["test"])
		graphAdapter.mu.RUnlock()
	})
}

// TestCollectPendingUpdates tests the collectPendingUpdates method.
func TestCollectPendingUpdates(t *testing.T) {
	t.Run("collects updates correctly", func(t *testing.T) {
		graphAdapter := &pregelGraphAdapter[[]message.Message, message.Message]{}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName:     "test",
			graphAdapter: graphAdapter,
		}

		updates := state.Updates{
			"key1": "value1",
			"key2": 42,
		}

		adapter.collectPendingUpdates(updates, nil)

		graphAdapter.updatesMu.Lock()
		defer graphAdapter.updatesMu.Unlock()

		assert.Len(t, graphAdapter.pendingUpdates, 2)

		// Check that both updates are present
		channels := make(map[string]bool)
		for _, pw := range graphAdapter.pendingUpdates {
			channels[pw.Channel] = true
			assert.Equal(t, "test", pw.NodeName)
			assert.False(t, pw.Timestamp.IsZero())
		}
		assert.True(t, channels["key1"])
		assert.True(t, channels["key2"])
	})

	t.Run("does nothing for empty updates", func(t *testing.T) {
		graphAdapter := &pregelGraphAdapter[[]message.Message, message.Message]{}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName:     "test",
			graphAdapter: graphAdapter,
		}

		adapter.collectPendingUpdates(nil, nil)
		adapter.collectPendingUpdates(state.Updates{}, nil)

		graphAdapter.updatesMu.Lock()
		defer graphAdapter.updatesMu.Unlock()
		assert.Empty(t, graphAdapter.pendingUpdates)
	})
}

// TestYieldOutputFromUpdates tests the yieldOutputFromUpdates method.
func TestYieldOutputFromUpdates(t *testing.T) {
	t.Run("yields wildcard output", func(t *testing.T) {
		var yielded state.Updates
		executor := &PregelExecutor[[]message.Message, state.Updates]{
			outputKey: "*",
			outputAdapter: func(v any) state.Updates {
				if u, ok := v.(state.Updates); ok {
					return u
				}
				return nil
			},
		}

		adapter := &pregelNodeAdapter[[]message.Message, state.Updates]{
			nodeName: "test",
			executor: executor,
			yield: func(output state.Updates, err error) bool {
				yielded = output
				return true
			},
		}

		updates := state.Updates{"key": "value"}
		result := adapter.yieldOutputFromUpdates(updates)

		assert.True(t, result)
		assert.Equal(t, updates, yielded)
	})

	t.Run("yields specific key value", func(t *testing.T) {
		var yielded message.Message
		executor := &PregelExecutor[[]message.Message, message.Message]{
			outputKey: MessagesKeyName,
			outputAdapter: func(v any) message.Message {
				if msg, ok := v.(message.Message); ok {
					return msg
				}
				return nil
			},
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
			yield: func(output message.Message, err error) bool {
				yielded = output
				return true
			},
		}

		msg := message.NewHumanMessageFromText("test")
		updates := state.Updates{MessagesKeyName: msg}
		result := adapter.yieldOutputFromUpdates(updates)

		assert.True(t, result)
		assert.Equal(t, msg, yielded)
	})

	t.Run("returns true when key not in updates", func(t *testing.T) {
		yieldCalled := false
		executor := &PregelExecutor[[]message.Message, message.Message]{
			outputKey: MessagesKeyName,
			outputAdapter: func(v any) message.Message {
				if msg, ok := v.(message.Message); ok {
					return msg
				}
				return nil
			},
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: executor,
			yield: func(output message.Message, err error) bool {
				yieldCalled = true
				return true
			},
		}

		updates := state.Updates{"other_key": "value"}
		result := adapter.yieldOutputFromUpdates(updates)

		assert.True(t, result)
		assert.False(t, yieldCalled)
	})
}

// TestHandleInterruptBefore tests the handleInterruptBefore method.
func TestHandleInterruptBefore(t *testing.T) {
	t.Run("returns false when resuming", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})
		compiled.graph.InterruptBefore = []string{"test"} // Enable interrupt

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		builder := state.NewManagerBuilder()
		mgr := builder.Build()
		ctx := context.Background()
		view, _ := mgr.CreateReadView(ctx)

		result := adapter.handleInterruptBefore(ctx, view, true) // isResuming = true
		assert.False(t, result)
	})

	t.Run("returns false when not in interrupt list", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})
		// Don't add to InterruptBefore

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		builder := state.NewManagerBuilder()
		mgr := builder.Build()
		ctx := context.Background()
		view, _ := mgr.CreateReadView(ctx)

		result := adapter.handleInterruptBefore(ctx, view, false)
		assert.False(t, result)
	})

	t.Run("returns true and pauses when interrupt needed", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})
		compiled.graph.InterruptBefore = []string{"test"}

		metrics := NewRuntimeMetrics()
		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: metrics,
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
			executor: executor,
		}

		builder := state.NewManagerBuilder()
		mgr := builder.Build()
		ctx := context.Background()
		view, _ := mgr.CreateReadView(ctx)

		result := adapter.handleInterruptBefore(ctx, view, false)
		assert.True(t, result)

		// Check node is paused
		assert.True(t, adapter.checkPauseState())
	})
}

// TestHandleInterruptAfter tests the handleInterruptAfter method.
func TestHandleInterruptAfter(t *testing.T) {
	t.Run("returns false when not in interrupt list", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})
		// Don't add to InterruptAfter

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		builder := state.NewManagerBuilder()
		mgr := builder.Build()
		ctx := context.Background()
		view, _ := mgr.CreateReadView(ctx)

		result := adapter.handleInterruptAfter(ctx, view, nil)
		assert.False(t, result)
	})

	t.Run("returns true and pauses when interrupt needed", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})
		compiled.graph.InterruptAfter = []string{"test"}

		metrics := NewRuntimeMetrics()
		executor := &PregelExecutor[[]message.Message, message.Message]{
			metrics: metrics,
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
			executor: executor,
		}

		builder := state.NewManagerBuilder()
		mgr := builder.Build()
		ctx := context.Background()
		view, _ := mgr.CreateReadView(ctx)

		updates := state.Updates{"key": "value"}
		result := adapter.handleInterruptAfter(ctx, view, updates)
		assert.True(t, result)

		// Check node is paused
		assert.True(t, adapter.checkPauseState())
	})
}

// TestGetInterruptStage tests the getInterruptStage method.
func TestGetInterruptStage(t *testing.T) {
	adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
		nodeName: "test",
	}

	t.Run("returns before for isBefore=true", func(t *testing.T) {
		result := adapter.getInterruptStage(true)
		assert.Equal(t, "before", result)
	})

	t.Run("returns after for isBefore=false", func(t *testing.T) {
		result := adapter.getInterruptStage(false)
		assert.Equal(t, "after", result)
	})
}

// TestCreateApprovalMetadata tests the createApprovalMetadata method.
func TestCreateApprovalMetadata(t *testing.T) {
	t.Run("creates metadata without timeout", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		vsnap := &state.VersionedSnapshot{
			Timestamp: time.Now(),
			Data:      state.Updates{"key": "value"},
		}

		metadata := adapter.createApprovalMetadata(vsnap, "test reason")

		assert.NotNil(t, metadata)
		assert.Len(t, metadata.PendingApprovals, 1)
		assert.Equal(t, "test", metadata.PendingApprovals["test"].NodeName)
		assert.Equal(t, "test reason", metadata.PendingApprovals["test"].Reason)
		assert.Nil(t, metadata.PendingApprovals["test"].TimeoutAt)
	})

	t.Run("creates metadata with timeout from config", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		// Add approval config with timeout
		compiled.graph.ApprovalConfigs = map[string]*ApprovalConfig{
			"test": {
				Timeout: 5 * time.Minute,
			},
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		vsnap := &state.VersionedSnapshot{
			Timestamp: time.Now(),
			Data:      state.Updates{"key": "value"},
		}

		metadata := adapter.createApprovalMetadata(vsnap, "test reason")

		assert.NotNil(t, metadata)
		assert.NotNil(t, metadata.PendingApprovals["test"].TimeoutAt)
	})
}

// TestStartObservability tests the startObservability method.
func TestStartObservability(t *testing.T) {
	t.Run("returns context and cleanup function", func(t *testing.T) {
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
		}

		ctx := context.Background()
		newCtx, endFunc := adapter.startObservability(ctx)

		assert.NotNil(t, newCtx)
		assert.NotNil(t, endFunc)

		// Should not panic
		endFunc(nil)
		endFunc(errors.New("test error"))
	})
}

// TestExecuteNodeLogic tests the executeNodeLogic method.
func TestExecuteNodeLogic(t *testing.T) {
	t.Run("returns tuple from successful execution", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, state.Updates{"key": "value"}, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		builder := state.NewManagerBuilder()
		mgr := builder.Build()
		ctx := context.Background()
		view, _ := mgr.CreateReadView(ctx)

		targets, updates, err := adapter.executeNodeLogic(ctx, testNode, view)

		assert.NoError(t, err)
		assert.Equal(t, []string{EndNode}, targets)
		assert.Equal(t, "value", updates["key"])
	})

	t.Run("wraps errors in NodeExecutionError", func(t *testing.T) {
		expectedErr := errors.New("test error")
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return nil, nil, expectedErr
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		builder := state.NewManagerBuilder()
		mgr := builder.Build()
		ctx := context.Background()
		view, _ := mgr.CreateReadView(ctx)

		targets, updates, err := adapter.executeNodeLogic(ctx, testNode, view)

		assert.Error(t, err)
		assert.Nil(t, targets)
		assert.Nil(t, updates)

		var nodeErr *NodeExecutionError
		assert.True(t, errors.As(err, &nodeErr))
		assert.Equal(t, "test", nodeErr.NodeName)
	})
}

// TestGetRequiredState tests the getRequiredState method.
func TestGetRequiredState(t *testing.T) {
	t.Run("returns nil when no config", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		vsnap := &state.VersionedSnapshot{
			Data: state.Updates{"key1": "value1", "key2": "value2"},
		}

		result := adapter.getRequiredState(vsnap)
		assert.Nil(t, result)
	})

	t.Run("filters state to requested keys", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		compiled.graph.ApprovalConfigs = map[string]*ApprovalConfig{
			"test": {
				StateSnapshot: []string{"key1"},
			},
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		vsnap := &state.VersionedSnapshot{
			Data: state.Updates{"key1": "value1", "key2": "value2"},
		}

		result := adapter.getRequiredState(vsnap)
		assert.Len(t, result, 1)
		assert.Equal(t, "value1", result["key1"])
	})

	t.Run("returns all state when StateSnapshot is empty", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		compiled.graph.ApprovalConfigs = map[string]*ApprovalConfig{
			"test": {
				StateSnapshot: []string{}, // Empty means all state
			},
		}

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			compiled: compiled,
		}

		vsnap := &state.VersionedSnapshot{
			Data: state.Updates{"key1": "value1", "key2": "value2"},
		}

		result := adapter.getRequiredState(vsnap)
		assert.Len(t, result, 2)
	})
}

// TestCreateInterruptCheckpoint tests the createInterruptCheckpoint method.
func TestCreateInterruptCheckpoint(t *testing.T) {
	t.Run("does nothing without executor", func(t *testing.T) {
		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName: "test",
			executor: nil,
		}

		// Should not panic
		adapter.createInterruptCheckpoint(context.Background(), nil, true, "reason")
	})
}

// TestApplyIncomingDistributedState tests the applyIncomingDistributedState method.
func TestApplyIncomingDistributedState(t *testing.T) {
	t.Run("does nothing when distributed state disabled", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName:               "test",
			compiled:               compiled,
			enableDistributedState: false,
		}

		incoming := []pregel.Message[state.Updates]{
			{From: "source", To: "test", Data: state.Updates{"key": "value"}},
		}

		err := adapter.applyIncomingDistributedState(context.Background(), incoming)
		assert.NoError(t, err)
	})

	t.Run("does nothing when no incoming messages", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName:               "test",
			compiled:               compiled,
			enableDistributedState: true,
		}

		err := adapter.applyIncomingDistributedState(context.Background(), nil)
		assert.NoError(t, err)
	})

	t.Run("skips empty messages", func(t *testing.T) {
		testNode := &BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, nil, nil
			},
		}
		compiled := createTestCompiledGraph(t, map[string]Node{"test": testNode})

		adapter := &pregelNodeAdapter[[]message.Message, message.Message]{
			nodeName:               "test",
			compiled:               compiled,
			enableDistributedState: true,
		}

		incoming := []pregel.Message[state.Updates]{
			{From: "source", To: "test", Data: nil},
			{From: "source2", To: "test", Data: state.Updates{}},
		}

		err := adapter.applyIncomingDistributedState(context.Background(), incoming)
		assert.NoError(t, err)
	})
}
