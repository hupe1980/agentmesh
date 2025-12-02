package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/pregel"
)

// DistributedBackend abstracts the underlying distributed execution mechanism.
// This interface hides Pregel implementation details from graph-layer users,
// allowing them to work with distributed execution without understanding
// supersteps, vertices, or message buses.
//
// The graph package provides adapters to convert common backends (Redis, gRPC, etc.)
// into this interface, keeping the graph API clean and implementation-agnostic.
type DistributedBackend interface {
	// Send delivers state updates to target nodes in a distributed execution.
	// This abstracts away the underlying message-passing mechanism.
	Send(ctx context.Context, updates []StateUpdate) error

	// Receive retrieves pending state updates for a specific node.
	// Returns nil if no updates are pending.
	Receive(node string) ([]StateUpdate, error)

	// Clear removes all pending updates for a node.
	Clear(node string) error

	// Close releases resources held by the backend.
	Close() error
}

// StateUpdate represents a state change being sent between nodes.
// This is the graph-layer equivalent of a Pregel message.
type StateUpdate struct {
	From string  // Source node
	To   string  // Target node
	Data Updates // State changes
}

// pregelBackendAdapter adapts a Pregel MessageBus to the DistributedBackend interface.
// This is an internal adapter that bridges the graph and Pregel layers.
type pregelBackendAdapter struct {
	bus pregel.MessageBus[Updates]
}

// NewPregelBackend creates a DistributedBackend from a Pregel MessageBus.
// This adapter allows existing Pregel message buses (Redis, in-memory, etc.)
// to work with the graph-layer API without exposing Pregel types to users.
func NewPregelBackend(bus pregel.MessageBus[Updates]) DistributedBackend {
	return &pregelBackendAdapter{bus: bus}
}

func (a *pregelBackendAdapter) Send(ctx context.Context, updates []StateUpdate) error {
	// Convert graph StateUpdates to Pregel Messages
	messages := make([]pregel.Message[Updates], len(updates))
	for i, update := range updates {
		messages[i] = pregel.Message[Updates]{
			From: update.From,
			To:   update.To,
			Data: update.Data,
		}
	}
	return a.bus.Send(ctx, messages)
}

func (a *pregelBackendAdapter) Receive(node string) ([]StateUpdate, error) {
	messages, err := a.bus.Receive(node)
	if err != nil {
		return nil, err
	}
	if messages == nil {
		return nil, nil
	}

	// Convert Pregel Messages to graph StateUpdates
	updates := make([]StateUpdate, len(messages))
	for i, msg := range messages {
		updates[i] = StateUpdate{
			From: msg.From,
			To:   msg.To,
			Data: msg.Data,
		}
	}
	return updates, nil
}

func (a *pregelBackendAdapter) Clear(node string) error {
	return a.bus.Clear(node)
}

func (a *pregelBackendAdapter) Close() error {
	return a.bus.Close()
}

// backendToMessageBusAdapter adapts a DistributedBackend back to Pregel MessageBus.
// This internal adapter allows the executor to use the graph-layer backend
// with the underlying Pregel runtime without exposing Pregel types externally.
type backendToMessageBusAdapter struct {
	backend DistributedBackend
}

func (a *backendToMessageBusAdapter) Send(ctx context.Context, messages []pregel.Message[Updates]) error {
	// Convert Pregel Messages to graph StateUpdates
	updates := make([]StateUpdate, len(messages))
	for i, msg := range messages {
		updates[i] = StateUpdate{
			From: msg.From,
			To:   msg.To,
			Data: msg.Data,
		}
	}
	return a.backend.Send(ctx, updates)
}

func (a *backendToMessageBusAdapter) Receive(vertex string) ([]pregel.Message[Updates], error) {
	updates, err := a.backend.Receive(vertex)
	if err != nil {
		return nil, err
	}
	if updates == nil {
		return nil, nil
	}

	// Convert graph StateUpdates to Pregel Messages
	messages := make([]pregel.Message[Updates], len(updates))
	for i, update := range updates {
		messages[i] = pregel.Message[Updates]{
			From: update.From,
			To:   update.To,
			Data: update.Data,
		}
	}
	return messages, nil
}

func (a *backendToMessageBusAdapter) Clear(vertex string) error {
	return a.backend.Clear(vertex)
}

func (a *backendToMessageBusAdapter) Close() error {
	return a.backend.Close()
}
