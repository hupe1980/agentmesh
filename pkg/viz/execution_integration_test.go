package viz

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionControl_FullIntegration(t *testing.T) {
	// This is a simplified test that verifies the controller is created and attached
	// Full end-to-end testing requires a real graph execution which is complex

	// Create server
	server, err := NewServer(Config{})
	require.NoError(t, err)

	ctx := context.Background()
	controller := NewExecutionController(ctx, "test-run")

	// Store controller
	server.mu.Lock()
	server.executionControllers["test-run"] = controller
	server.mu.Unlock()

	// Verify retrieval
	server.mu.RLock()
	retrieved := server.executionControllers["test-run"]
	server.mu.RUnlock()

	assert.NotNil(t, retrieved)
	assert.Equal(t, controller, retrieved)

	// Cleanup
	server.mu.Lock()
	delete(server.executionControllers, "test-run")
	server.mu.Unlock()
}

func TestExecutionControl_Breakpoint(t *testing.T) {
	ctx := context.Background()
	controller := NewExecutionController(ctx, "test-run")

	// Add node breakpoint
	controller.AddBreakpoint(&Breakpoint{
		Type: BreakpointNode,
		Node: "test-node",
	})

	// Check breakpoint hits
	hit := controller.CheckBreakpoint("test-node", 1, false)
	assert.True(t, hit)
	assert.Equal(t, StatePaused, controller.GetState())

	// Resume
	controller.SetState(StateRunning)

	// Check different node doesn't hit
	hit = controller.CheckBreakpoint("other-node", 1, false)
	assert.False(t, hit)
}

func TestExecutionControl_StepMode(t *testing.T) {
	ctx := context.Background()
	controller := NewExecutionController(ctx, "test-run")

	// Enable step mode
	controller.SetStepMode(true, 0)
	assert.Equal(t, StateSteppingStep, controller.GetState())

	// Set current superstep
	controller.CheckBreakpoint("node1", 1, false)

	// Should pause on next superstep
	shouldPause := controller.ShouldPause("node2", 2)
	assert.True(t, shouldPause)

	// Should not pause on same superstep
	shouldPause = controller.ShouldPause("node2", 1)
	assert.False(t, shouldPause)
}

func TestExecutionControl_Commands(t *testing.T) {
	ctx := context.Background()
	controller := NewExecutionController(ctx, "test-run")

	// Send pause command
	err := controller.SendCommand(CommandPause)
	require.NoError(t, err)

	// Receive command
	done := make(chan ExecutionCommand)
	go func() {
		cmd, _ := controller.WaitForCommand()
		done <- cmd
	}()

	select {
	case cmd := <-done:
		assert.Equal(t, CommandPause, cmd)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for command")
	}
}

func TestExecutionControl_ContextIntegration(t *testing.T) {
	ctx := context.Background()
	controller := NewExecutionController(ctx, "test-run")

	// Attach to context
	ctx = WithExecutionController(ctx, controller)

	// Retrieve from context
	retrieved := ExecutionControllerFromContext(ctx)
	assert.NotNil(t, retrieved)
	assert.Equal(t, controller, retrieved)

	// Check nil context
	empty := context.Background()
	retrieved = ExecutionControllerFromContext(empty)
	assert.Nil(t, retrieved)
}

func TestExecutionInterceptor_HandleEvent(t *testing.T) {
	// Create server and handler
	server, err := NewServer(Config{})
	require.NoError(t, err)

	handler := NewGraphEventHandler(server, "test-run")
	interceptor := NewExecutionInterceptor(handler)

	ctx := context.Background()
	controller := NewExecutionController(ctx, "test-run")
	ctx = WithExecutionController(ctx, controller)

	// Add node breakpoint
	controller.AddBreakpoint(&Breakpoint{
		Type: BreakpointNode,
		Node: "test-node",
	})

	// Create node start event
	event := graph.Event{
		Type:      graph.EventNodeStart,
		Node:      "test-node",
		Superstep: 1,
	}

	// Handle event in goroutine (will block waiting for command)
	done := make(chan error)
	go func() {
		done <- interceptor.HandleEvent(ctx, event)
	}()

	// Give it time to hit breakpoint
	time.Sleep(50 * time.Millisecond)

	// Check state is paused
	assert.Equal(t, StatePaused, controller.GetState())

	// Send resume command
	err = controller.SendCommand(CommandResume)
	require.NoError(t, err)

	// Wait for handler to complete
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for handler")
	}

	// Check state is running
	assert.Equal(t, StateRunning, controller.GetState())
}

func TestExecutionInterceptor_NoController(t *testing.T) {
	// Create server and handler
	server, err := NewServer(Config{})
	require.NoError(t, err)

	handler := NewGraphEventHandler(server, "test-run")
	interceptor := NewExecutionInterceptor(handler)

	// No controller in context
	ctx := context.Background()

	// Create event
	event := graph.Event{
		Type:      graph.EventNodeStart,
		Node:      "test-node",
		Superstep: 1,
	}

	// Should pass through without blocking
	err = interceptor.HandleEvent(ctx, event)
	assert.NoError(t, err)
}
