package viz

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// ExecutionInterceptor wraps graph event handling to add execution control.
// It checks for breakpoints and pause conditions during execution.
type ExecutionInterceptor struct {
	handler *GraphEventHandler
}

// NewExecutionInterceptor creates a new interceptor that adds execution control.
func NewExecutionInterceptor(handler *GraphEventHandler) *ExecutionInterceptor {
	return &ExecutionInterceptor{
		handler: handler,
	}
}

// HandleEvent intercepts graph events to add execution control checks.
func (i *ExecutionInterceptor) HandleEvent(ctx context.Context, event graph.Event) error {
	// Get execution controller from context
	controller := ExecutionControllerFromContext(ctx)

	// If no controller, just pass through to normal handler
	if controller == nil {
		return i.handler.HandleEvent(ctx, event)
	}

	// Check for pause/breakpoint conditions based on event type
	switch event.Type {
	case graph.EventNodeStart:
		// Check breakpoints before node execution
		if err := i.checkBreakpoint(controller, event); err != nil {
			return err
		}

	case graph.EventSuperstepStart:
		// Check for pause at superstep boundaries
		if err := i.checkSuperstepPause(controller, event); err != nil {
			return err
		}

	case graph.EventNodeError:
		// Check error breakpoints
		if controller.CheckBreakpoint(event.Node, int64(event.Superstep), true) {
			i.broadcastBreakpointHit(controller, event, "error")
			if err := i.waitForCommand(controller); err != nil {
				return err
			}
		}
	}

	// Forward to original handler
	return i.handler.HandleEvent(ctx, event)
}

// checkBreakpoint checks if execution should pause at a node breakpoint
func (i *ExecutionInterceptor) checkBreakpoint(controller *ExecutionController, event graph.Event) error {
	if controller.CheckBreakpoint(event.Node, int64(event.Superstep), false) {
		// Broadcast breakpoint hit
		i.broadcastBreakpointHit(controller, event, "node")

		// Wait for command (resume, step, etc.)
		return i.waitForCommand(controller)
	}
	return nil
}

// checkSuperstepPause checks if execution should pause at superstep boundary
func (i *ExecutionInterceptor) checkSuperstepPause(controller *ExecutionController, event graph.Event) error {
	if controller.ShouldPause("", int64(event.Superstep)) {
		// Broadcast pause
		i.handler.server.wsHub.BroadcastMessage(Message{
			Type:  "control",
			RunID: i.handler.runID,
			Data: map[string]any{
				"event":     "paused",
				"state":     controller.GetState(),
				"superstep": event.Superstep,
				"reason":    "step_mode",
			},
		})

		// Wait for command
		return i.waitForCommand(controller)
	}
	return nil
}

// waitForCommand waits for a control command and handles it
func (i *ExecutionInterceptor) waitForCommand(controller *ExecutionController) error {
	for {
		cmd, err := controller.WaitForCommand()
		if err != nil {
			return fmt.Errorf("execution control error: %w", err)
		}

		switch cmd {
		case CommandResume, CommandContinue:
			controller.SetState(StateRunning)
			i.broadcastStateChange(controller, "resumed")
			return nil

		case CommandStep, CommandStepNode:
			controller.SetState(StateRunning)
			i.broadcastStateChange(controller, "stepping")
			return nil

		case CommandStop:
			controller.Stop()
			i.broadcastStateChange(controller, "stopped")
			return fmt.Errorf("execution stopped by user")

		case CommandPause:
			// Already paused, wait for next command
			continue

		default:
			// Unknown command, ignore and wait
			continue
		}
	}
}

// broadcastBreakpointHit sends a breakpoint hit notification via WebSocket
func (i *ExecutionInterceptor) broadcastBreakpointHit(controller *ExecutionController, event graph.Event, breakpointType string) {
	node, superstep := controller.GetCurrentPosition()

	i.handler.server.wsHub.BroadcastMessage(Message{
		Type:  "control",
		RunID: i.handler.runID,
		Data: map[string]any{
			"event":           "breakpoint_hit",
			"state":           controller.GetState(),
			"breakpoint_type": breakpointType,
			"node":            node,
			"superstep":       superstep,
			"event_type":      string(event.Type),
		},
	})
}

// broadcastStateChange sends a state change notification via WebSocket
func (i *ExecutionInterceptor) broadcastStateChange(controller *ExecutionController, event string) {
	node, superstep := controller.GetCurrentPosition()

	i.handler.server.wsHub.BroadcastMessage(Message{
		Type:  "control",
		RunID: i.handler.runID,
		Data: map[string]any{
			"event":     event,
			"state":     controller.GetState(),
			"node":      node,
			"superstep": superstep,
		},
	})
}
