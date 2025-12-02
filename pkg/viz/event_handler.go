package viz

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/event"
)

// GraphEventHandler adapts viz to work with the graph event bus.
// It converts graph events to viz events and stores/broadcasts them.
type GraphEventHandler struct {
	server       *Server
	runID        string
	stateTracker *GraphStateTracker
}

// NewGraphEventHandler creates a new event handler for visualization.
func NewGraphEventHandler(server *Server, runID string) *GraphEventHandler {
	return &GraphEventHandler{
		server:       server,
		runID:        runID,
		stateTracker: NewGraphStateTracker(runID),
	}
}

// HandleEvent implements event.Handler interface.
func (h *GraphEventHandler) HandleEvent(ctx context.Context, e event.Event) error {
	// Convert graph event to enhanced ExecutionEvent
	vizEvent := ConvertGraphEvent(e, h.runID)

	// Update graph state and get visualization update
	h.updateGraphState(e, &vizEvent)

	// Store event
	if err := h.server.eventStore.Append(vizEvent); err != nil {
		return err
	}

	// Broadcast via WebSocket
	h.server.wsHub.BroadcastMessage(Message{
		Type:  "event",
		RunID: h.runID,
		Data:  vizEvent,
	})

	return nil
}

// updateGraphState updates the graph visualization state based on the event.
func (h *GraphEventHandler) updateGraphState(e event.Event, vizEvent *ExecutionEvent) {
	var stateUpdate *GraphStateUpdate

	switch e.Type {
	case event.EventNodeQueued:
		vizEvent.NodeStatus = NodeStatusQueued
		stateUpdate = h.stateTracker.UpdateNodeStatus(e.Node, NodeStatusQueued, e.Superstep)

	case event.EventNodeStart:
		vizEvent.NodeStatus = NodeStatusActive
		stateUpdate = h.stateTracker.UpdateNodeStatus(e.Node, NodeStatusActive, e.Superstep)

	case event.EventNodeComplete:
		vizEvent.NodeStatus = NodeStatusCompleted
		stateUpdate = h.stateTracker.UpdateNodeStatus(e.Node, NodeStatusCompleted, e.Superstep)

		// Track edge traversals (node -> next nodes)
		if nextNodes, ok := e.Data["next_nodes"].([]string); ok {
			for _, nextNode := range nextNodes {
				edgeUpdate := h.stateTracker.AddEdgeTraversal(e.Node, nextNode, e.Superstep)
				if edgeUpdate != nil {
					h.broadcastStateUpdate(edgeUpdate)
				}
			}
		}

	case event.EventNodeError:
		vizEvent.NodeStatus = NodeStatusError
		stateUpdate = h.stateTracker.UpdateNodeStatus(e.Node, NodeStatusError, e.Superstep)

	case event.EventInterrupt:
		if e.Node != "" {
			vizEvent.NodeStatus = NodeStatusPaused
			stateUpdate = h.stateTracker.UpdateNodeStatus(e.Node, NodeStatusPaused, e.Superstep)
		}

	case event.EventStateUpdate:
		// Extract state keys if available
		if keys, ok := e.Data["keys"].([]string); ok {
			sizeBytes := 0
			if size, ok := e.Data["size"].(int); ok {
				sizeBytes = size
			}
			stateUpdate = h.stateTracker.UpdateState(keys, sizeBytes)
		}
	}

	// Broadcast state update if there was a change
	if stateUpdate != nil {
		h.broadcastStateUpdate(stateUpdate)
	}
}

// broadcastStateUpdate sends a graph state update via WebSocket.
func (h *GraphEventHandler) broadcastStateUpdate(update *GraphStateUpdate) {
	h.server.wsHub.BroadcastMessage(Message{
		Type:  "graph_state",
		RunID: h.runID,
		Data:  update,
	})
}

// SubscribeToGraph subscribes this handler to the graph event bus.
// If no event bus exists in the context, one is created automatically.
// Returns the context with the event bus attached (reusing existing or creating new).
func (h *GraphEventHandler) SubscribeToGraph(ctx context.Context) context.Context {
	eventBus := event.BusFromContext(ctx)
	if eventBus == nil {
		// Create event bus on demand
		eventBus = event.NewBus()
		ctx = event.WithBus(ctx, eventBus)
	}

	// Subscribe to all events (no type filter = receive everything)
	eventBus.Subscribe(h)

	return ctx
}
