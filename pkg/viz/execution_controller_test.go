package viz

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionController_NewExecutionController(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	assert.NotNil(t, ec)
	assert.Equal(t, "test-run", ec.runID)
	assert.Equal(t, StateRunning, ec.GetState())
	assert.NotNil(t, ec.ctx)
	assert.NotNil(t, ec.commandQueue)
	assert.NotNil(t, ec.breakpoints)
}

func TestExecutionController_GetSetState(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Initial state
	assert.Equal(t, StateRunning, ec.GetState())

	// Change state
	ec.SetState(StatePaused)
	assert.Equal(t, StatePaused, ec.GetState())

	// Change to another state
	ec.SetState(StateStopped)
	assert.Equal(t, StateStopped, ec.GetState())
}

func TestExecutionController_SendCommand(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Send command while running
	err := ec.SendCommand(CommandPause)
	require.NoError(t, err)

	// Receive command
	select {
	case cmd := <-ec.commandQueue:
		assert.Equal(t, CommandPause, cmd)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for command")
	}
}

func TestExecutionController_SendCommand_WhenStopped(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Stop execution
	ec.SetState(StateStopped)

	// Try to send command
	err := ec.SendCommand(CommandResume)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot send command")
}

func TestExecutionController_SendCommand_WhenCompleted(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Set state to completed
	ec.SetState(StateCompleted)

	// Try to send command
	err := ec.SendCommand(CommandResume)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot send command")
}

func TestExecutionController_WaitForCommand(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Send command in goroutine
	go func() {
		time.Sleep(50 * time.Millisecond)
		ec.SendCommand(CommandPause)
	}()

	// Wait for command
	cmd, err := ec.WaitForCommand()
	require.NoError(t, err)
	assert.Equal(t, CommandPause, cmd)
}

func TestExecutionController_WaitForCommand_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ec := NewExecutionController(ctx, "test-run")

	// Cancel context
	cancel()

	// Wait for command (should return error)
	_, err := ec.WaitForCommand()
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestExecutionController_AddBreakpoint(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	bp := &Breakpoint{
		Type: BreakpointNode,
		Node: "test-node",
	}

	ec.AddBreakpoint(bp)

	// Verify breakpoint was added
	breakpoints := ec.GetBreakpoints()
	assert.Len(t, breakpoints, 1)
	assert.Equal(t, BreakpointNode, breakpoints[0].Type)
	assert.Equal(t, "test-node", breakpoints[0].Node)
	assert.True(t, breakpoints[0].Enabled)
	assert.NotEmpty(t, breakpoints[0].ID)
}

func TestExecutionController_RemoveBreakpoint(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	bp := &Breakpoint{
		ID:   "bp1",
		Type: BreakpointNode,
		Node: "test-node",
	}

	ec.AddBreakpoint(bp)

	// Verify it exists
	assert.Len(t, ec.GetBreakpoints(), 1)

	// Remove it
	err := ec.RemoveBreakpoint("bp1")
	require.NoError(t, err)

	// Verify it's gone
	assert.Len(t, ec.GetBreakpoints(), 0)
}

func TestExecutionController_RemoveBreakpoint_NotFound(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	err := ec.RemoveBreakpoint("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecutionController_EnableBreakpoint(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	bp := &Breakpoint{
		ID:   "bp1",
		Type: BreakpointNode,
		Node: "test-node",
	}

	ec.AddBreakpoint(bp)

	// Initially enabled
	assert.True(t, ec.GetBreakpoints()[0].Enabled)

	// Disable it
	err := ec.EnableBreakpoint("bp1", false)
	require.NoError(t, err)
	assert.False(t, ec.GetBreakpoints()[0].Enabled)

	// Enable it again
	err = ec.EnableBreakpoint("bp1", true)
	require.NoError(t, err)
	assert.True(t, ec.GetBreakpoints()[0].Enabled)
}

func TestExecutionController_CheckBreakpoint_Node(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	bp := &Breakpoint{
		Type: BreakpointNode,
		Node: "test-node",
	}

	ec.AddBreakpoint(bp)

	// Should break on matching node
	shouldBreak := ec.CheckBreakpoint("test-node", 1, false)
	assert.True(t, shouldBreak)
	assert.Equal(t, StatePaused, ec.GetState())

	// Verify hit count
	breakpoints := ec.GetBreakpoints()
	assert.Equal(t, 1, breakpoints[0].HitCount)

	// Reset state
	ec.SetState(StateRunning)

	// Should not break on different node
	shouldBreak = ec.CheckBreakpoint("other-node", 1, false)
	assert.False(t, shouldBreak)
}

func TestExecutionController_CheckBreakpoint_Superstep(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	bp := &Breakpoint{
		Type:      BreakpointSuperstep,
		Superstep: 5,
	}

	ec.AddBreakpoint(bp)

	// Should not break before superstep
	shouldBreak := ec.CheckBreakpoint("any-node", 3, false)
	assert.False(t, shouldBreak)

	// Should break at superstep
	shouldBreak = ec.CheckBreakpoint("any-node", 5, false)
	assert.True(t, shouldBreak)
	assert.Equal(t, StatePaused, ec.GetState())
}

func TestExecutionController_CheckBreakpoint_Error(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	bp := &Breakpoint{
		Type: BreakpointError,
	}

	ec.AddBreakpoint(bp)

	// Should not break when no error
	shouldBreak := ec.CheckBreakpoint("any-node", 1, false)
	assert.False(t, shouldBreak)

	// Should break when error
	shouldBreak = ec.CheckBreakpoint("any-node", 1, true)
	assert.True(t, shouldBreak)
	assert.Equal(t, StatePaused, ec.GetState())
}

func TestExecutionController_CheckBreakpoint_Disabled(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	bp := &Breakpoint{
		ID:   "bp1",
		Type: BreakpointNode,
		Node: "test-node",
	}

	ec.AddBreakpoint(bp)

	// Disable breakpoint
	ec.EnableBreakpoint("bp1", false)

	// Should not break when disabled
	shouldBreak := ec.CheckBreakpoint("test-node", 1, false)
	assert.False(t, shouldBreak)
}

func TestExecutionController_MultipleBreakpoints(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Add multiple breakpoints
	ec.AddBreakpoint(&Breakpoint{Type: BreakpointNode, Node: "node1"})
	ec.AddBreakpoint(&Breakpoint{Type: BreakpointNode, Node: "node2"})
	ec.AddBreakpoint(&Breakpoint{Type: BreakpointSuperstep, Superstep: 5})

	breakpoints := ec.GetBreakpoints()
	assert.Len(t, breakpoints, 3)

	// Should break on node1
	shouldBreak := ec.CheckBreakpoint("node1", 1, false)
	assert.True(t, shouldBreak)

	ec.SetState(StateRunning)

	// Should break on node2
	shouldBreak = ec.CheckBreakpoint("node2", 1, false)
	assert.True(t, shouldBreak)

	ec.SetState(StateRunning)

	// Should break at superstep 5
	shouldBreak = ec.CheckBreakpoint("any-node", 5, false)
	assert.True(t, shouldBreak)
}

func TestExecutionController_SetStepMode(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Enable step mode
	ec.SetStepMode(true, 0)
	assert.Equal(t, StateSteppingStep, ec.GetState())

	// Check step mode is active
	ec.mu.RLock()
	assert.True(t, ec.stepMode)
	ec.mu.RUnlock()
}

func TestExecutionController_ShouldPause(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Set current step
	ec.currentStep = 3

	// Enable step mode
	ec.SetStepMode(true, 0)

	// Should pause on next step
	shouldPause := ec.ShouldPause("any-node", 4)
	assert.True(t, shouldPause)

	// Should not pause on same or earlier step
	shouldPause = ec.ShouldPause("any-node", 3)
	assert.False(t, shouldPause)
}

func TestExecutionController_ShouldPause_JumpTo(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Set jump target
	ec.SetStepMode(false, 10)

	// Should not pause before target
	shouldPause := ec.ShouldPause("any-node", 5)
	assert.False(t, shouldPause)

	// Should pause at or after target
	shouldPause = ec.ShouldPause("any-node", 10)
	assert.True(t, shouldPause)

	shouldPause = ec.ShouldPause("any-node", 11)
	assert.True(t, shouldPause)
}

func TestExecutionController_GetCurrentPosition(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	// Check breakpoint which sets position
	ec.CheckBreakpoint("test-node", 5, false)

	node, superstep := ec.GetCurrentPosition()
	assert.Equal(t, "test-node", node)
	assert.Equal(t, int64(5), superstep)
}

func TestExecutionController_Stop(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	ec.Stop()

	// Verify state is stopped
	assert.Equal(t, StateStopped, ec.GetState())

	// Verify context is cancelled
	select {
	case <-ec.Context().Done():
		// Context should be done
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context not cancelled")
	}

	// Verify command queue is closed
	_, ok := <-ec.commandQueue
	assert.False(t, ok, "command queue should be closed")
}

func TestExecutionController_ThreadSafety(t *testing.T) {
	ctx := context.Background()
	ec := NewExecutionController(ctx, "test-run")

	done := make(chan bool)
	iterations := 50

	// Concurrent state changes
	go func() {
		for i := 0; i < iterations; i++ {
			ec.SetState(StateRunning)
			ec.SetState(StatePaused)
		}
		done <- true
	}()

	// Concurrent breakpoint operations
	go func() {
		for i := 0; i < iterations; i++ {
			bp := &Breakpoint{Type: BreakpointNode, Node: "test"}
			ec.AddBreakpoint(bp)
			ec.GetBreakpoints()
		}
		done <- true
	}()

	// Concurrent command sends
	go func() {
		for i := 0; i < iterations; i++ {
			ec.SendCommand(CommandPause)
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// If we get here without data races, test passes
	assert.True(t, true)
}
