package viz

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
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

// HandleEvent implements graph.EventHandler interface.
func (h *GraphEventHandler) HandleEvent(ctx context.Context, event graph.Event) error {
	// Convert graph event to enhanced ExecutionEvent
	vizEvent := ConvertGraphEvent(event, h.runID)

	// Update graph state and get visualization update
	h.updateGraphState(event, &vizEvent)

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
func (h *GraphEventHandler) updateGraphState(event graph.Event, vizEvent *ExecutionEvent) {
	var stateUpdate *GraphStateUpdate

	switch event.Type {
	case graph.EventNodeQueued:
		vizEvent.NodeStatus = NodeStatusQueued
		stateUpdate = h.stateTracker.UpdateNodeStatus(event.Node, NodeStatusQueued, event.Superstep)

	case graph.EventNodeStart:
		vizEvent.NodeStatus = NodeStatusActive
		stateUpdate = h.stateTracker.UpdateNodeStatus(event.Node, NodeStatusActive, event.Superstep)

	case graph.EventNodeComplete:
		vizEvent.NodeStatus = NodeStatusCompleted
		stateUpdate = h.stateTracker.UpdateNodeStatus(event.Node, NodeStatusCompleted, event.Superstep)

		// Track edge traversals (node -> next nodes)
		if nextNodes, ok := event.Data["next_nodes"].([]string); ok {
			for _, nextNode := range nextNodes {
				edgeUpdate := h.stateTracker.AddEdgeTraversal(event.Node, nextNode, event.Superstep)
				if edgeUpdate != nil {
					h.broadcastStateUpdate(edgeUpdate)
				}
			}
		}

	case graph.EventNodeError:
		vizEvent.NodeStatus = NodeStatusError
		stateUpdate = h.stateTracker.UpdateNodeStatus(event.Node, NodeStatusError, event.Superstep)

	case graph.EventInterrupt:
		if event.Node != "" {
			vizEvent.NodeStatus = NodeStatusPaused
			stateUpdate = h.stateTracker.UpdateNodeStatus(event.Node, NodeStatusPaused, event.Superstep)
		}

	case graph.EventStateUpdate:
		// Extract state keys if available
		if keys, ok := event.Data["keys"].([]string); ok {
			sizeBytes := 0
			if size, ok := event.Data["size"].(int); ok {
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
	eventBus := graph.EventBusFromContext(ctx)
	if eventBus == nil {
		// Create event bus on demand
		eventBus = graph.NewEventBus()
		ctx = graph.WithEventBus(ctx, eventBus)
	}

	// Subscribe to all events (no type filter = receive everything)
	eventBus.Subscribe(h)

	return ctx
}
