package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInterrupt_BeforeNode tests that InterruptBefore stops execution before the node.
func TestInterrupt_BeforeNode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")
	var nodeExecuted bool

	g := graph.New(resultKey)
	g.Node("sensitive", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		nodeExecuted = true
		return graph.Set(resultKey, "executed").End()
	}, graph.END)
	g.InterruptBefore("sensitive")
	g.Start("sensitive")

	compiled, err := g.Build()
	require.NoError(t, err)

	var gotErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			gotErr = err
			break
		}
	}

	// Node should NOT have executed
	assert.False(t, nodeExecuted, "Node should not execute before approval")

	// Should get InterruptError
	require.Error(t, gotErr)
	var interruptErr *graph.InterruptError
	require.True(t, errors.As(gotErr, &interruptErr))
	assert.Equal(t, "sensitive", interruptErr.NodeName)
	assert.True(t, interruptErr.Before)
}

// TestInterrupt_AfterNode tests that InterruptAfter stops execution after the node.
func TestInterrupt_AfterNode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")
	var nodeExecuted bool

	g := graph.New(resultKey)
	g.Node("action", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		nodeExecuted = true
		return graph.Set(resultKey, "completed").End()
	}, graph.END)
	g.InterruptAfter("action")
	g.Start("action")

	compiled, err := g.Build()
	require.NoError(t, err)

	var gotErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			gotErr = err
			break
		}
	}

	// Node SHOULD have executed
	assert.True(t, nodeExecuted, "Node should execute before interrupt")

	// Should still get InterruptError
	require.Error(t, gotErr)
	var interruptErr *graph.InterruptError
	require.True(t, errors.As(gotErr, &interruptErr))
	assert.Equal(t, "action", interruptErr.NodeName)
	assert.False(t, interruptErr.Before)
}

// TestInterrupt_WithApproval tests that providing approval via Resume allows execution to continue.
func TestInterrupt_WithApproval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "approval-test"

	resultKey := graph.NewKey[string]("result")
	var nodeExecuted bool

	g := graph.New(resultKey)
	g.Node("sensitive", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		nodeExecuted = true
		return graph.Set(resultKey, "executed").End()
	}, graph.END)
	g.InterruptBefore("sensitive")
	g.Start("sensitive")
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	require.NoError(t, err)

	// First Run - will hit interrupt
	for _, err := range compiled.Run(ctx, nil, graph.WithRunID(runID)) {
		var intErr *graph.InterruptError
		if errors.As(err, &intErr) {
			// Expected interrupt
			break
		}
		require.NoError(t, err)
	}

	assert.False(t, nodeExecuted, "Node should not execute before approval")

	// Resume WITH approval
	approval := &graph.ApprovalResponse{
		Decision:  graph.ApprovalApproved,
		Timestamp: time.Now(),
	}

	for _, err := range compiled.Resume(ctx, runID, graph.WithApproval("sensitive", approval)) {
		require.NoError(t, err)
	}

	assert.True(t, nodeExecuted, "Node should execute when approval provided")
}

// TestInterrupt_WithRejection tests that rejecting via missing approval stops execution.
// NOTE: The current implementation doesn't distinguish between rejected and approved decisions.
// Rejection is achieved by not providing any approval.
func TestInterrupt_WithRejection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")
	var nodeExecuted bool

	g := graph.New(resultKey)
	g.Node("sensitive", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		nodeExecuted = true
		return graph.Set(resultKey, "executed").End()
	}, graph.END)
	g.InterruptBefore("sensitive")
	g.Start("sensitive")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Run WITHOUT providing any approval - this effectively rejects
	var gotErr error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			gotErr = err
			break
		}
	}

	assert.False(t, nodeExecuted, "Node should not execute without approval")
	require.Error(t, gotErr)

	var interruptErr *graph.InterruptError
	require.True(t, errors.As(gotErr, &interruptErr))
	assert.Equal(t, "sensitive", interruptErr.NodeName)
}

// TestInterrupt_MultipleNodes tests interrupts on multiple nodes in sequence.
// Each interrupt requires a separate Resume call with the appropriate approval.
func TestInterrupt_MultipleNodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")
	var node1Executed, node2Executed bool

	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "multi-interrupt-test"

	g := graph.New(resultKey)
	g.Node("step1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		node1Executed = true
		return graph.Set(resultKey, "step1").To("step2")
	}, "step2")

	g.Node("step2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		node2Executed = true
		return graph.Set(resultKey, "step2").End()
	}, graph.END)

	g.InterruptBefore("step1")
	g.InterruptBefore("step2")
	g.Start("step1")
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	require.NoError(t, err)

	// First run hits step1 interrupt
	var gotErr error
	for _, err := range compiled.Run(ctx, nil, graph.WithRunID(runID)) {
		if err != nil {
			gotErr = err
			break
		}
	}
	require.Error(t, gotErr)
	var interruptErr *graph.InterruptError
	require.True(t, errors.As(gotErr, &interruptErr))
	assert.Equal(t, "step1", interruptErr.NodeName)
	assert.False(t, node1Executed, "step1 should not execute before approval")

	// Resume with step1 approval - step1 executes, hits step2 interrupt
	approval1 := &graph.ApprovalResponse{Decision: graph.ApprovalApproved, Timestamp: time.Now()}
	for _, err := range compiled.Resume(ctx, runID, graph.WithApproval("step1", approval1)) {
		if err != nil {
			gotErr = err
			break
		}
	}
	require.Error(t, gotErr)
	require.True(t, errors.As(gotErr, &interruptErr))
	assert.Equal(t, "step2", interruptErr.NodeName)
	assert.True(t, node1Executed, "step1 should have executed")
	assert.False(t, node2Executed, "step2 should not execute before approval")

	// Resume with step2 approval - step2 executes, completes
	approval2 := &graph.ApprovalResponse{Decision: graph.ApprovalApproved, Timestamp: time.Now()}
	for _, err := range compiled.Resume(ctx, runID, graph.WithApproval("step2", approval2)) {
		require.NoError(t, err)
	}

	assert.True(t, node1Executed, "step1 should have executed")
	assert.True(t, node2Executed, "step2 should have executed")
}

// TestInterrupt_ChainWithPartialApproval tests chain where execution stops at unapproved node.
func TestInterrupt_ChainWithPartialApproval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resultKey := graph.NewKey[string]("result")
	var node1Executed, node2Executed bool

	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "partial-approval-test"

	g := graph.New(resultKey)
	g.Node("step1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		node1Executed = true
		return graph.Set(resultKey, "step1").To("step2")
	}, "step2")

	g.Node("step2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		node2Executed = true
		return graph.Set(resultKey, "step2").End()
	}, graph.END)

	g.InterruptBefore("step1")
	g.InterruptBefore("step2")
	g.Start("step1")
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	require.NoError(t, err)

	// First run hits step1 interrupt
	var gotErr error
	for _, err := range compiled.Run(ctx, nil, graph.WithRunID(runID)) {
		if err != nil {
			gotErr = err
			break
		}
	}
	require.Error(t, gotErr)
	var interruptErr *graph.InterruptError
	require.True(t, errors.As(gotErr, &interruptErr))
	assert.Equal(t, "step1", interruptErr.NodeName)

	// Resume with step1 approval only - step1 executes, step2 interrupts
	approval1 := &graph.ApprovalResponse{Decision: graph.ApprovalApproved, Timestamp: time.Now()}
	for _, err := range compiled.Resume(ctx, runID, graph.WithApproval("step1", approval1)) {
		if err != nil {
			gotErr = err
			break
		}
	}

	// step1 should execute, step2 should interrupt (no approval provided)
	assert.True(t, node1Executed, "step1 should execute with approval")
	assert.False(t, node2Executed, "step2 should not execute without approval")

	require.Error(t, gotErr)
	require.True(t, errors.As(gotErr, &interruptErr))
	assert.Equal(t, "step2", interruptErr.NodeName)
}
