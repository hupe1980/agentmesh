// Package event provides a unified event system for publishing and subscribing to
// execution events across all components (graph, model, tool, node).
//
// # Overview
//
// The event package solves the circular dependency problem by providing a
// low-level event infrastructure that can be imported by all packages:
//   - pkg/graph can publish graph and node events
//   - pkg/model can publish model events
//   - pkg/tool can publish tool events
//   - pkg/viz can subscribe to all events for visualization
//
// # Architecture
//
// The event system consists of:
//   - Bus: Manages subscriptions and publishes events synchronously without holding locks
//   - Event: Unified event structure with type, component, source, and data
//   - EventHandler: Interface for processing events
//   - Context helpers: Attach/retrieve event bus from context
//
// # Usage
//
// Publishing events:
//
//	import "github.com/hupe1980/agentmesh/pkg/event"
//
//	// Publish a model event
//	event.Publish(ctx, event.Event{
//	    Type: event.EventModelStart,
//	    Component: event.ComponentModel,
//	    Source: "gpt-4",
//	    RunID: runID,
//	    Data: map[string]any{
//	        "model": "gpt-4",
//	        "messages": 3,
//	    },
//	})
//
// Subscribing to events:
//
//	bus := event.NewBus()
//	bus.Subscribe(myHandler)
//	ctx = event.WithBus(ctx, bus)
//
//	// Now all Publish() calls will deliver to myHandler
//
// # Event Types
//
// Events are categorized by component:
//   - Graph: graph.start, graph.complete, graph.error
//   - Node: node.start, node.complete, node.error
//   - Model: model.start, model.complete, model.error, model.stream
//   - Tool: tool.start, tool.complete, tool.error
//   - Execution: superstep.start, superstep.complete, state.update
//
// # Synchronous Delivery
//
// Events are delivered synchronously to handlers in subscription order after the
// bus snapshots its handler lists. The snapshotting ensures:
//   - Handlers execute without holding internal locks, so slow subscribers do not block publishers
//   - Events remain ordered and deterministic
//   - Subscribers added during a publish do not see in-flight events (consistent snapshot)
//   - A simple mental model for debugging without goroutine fan-out
//
// # Design Decisions
//
// 1. Synchronous over async: Simpler, more reliable, minimal performance cost
// 2. Single Event struct: Works for all components, easy to extend
// 3. Component field: Identifies event source for filtering and routing
// 4. No circular deps: Event package can be imported by all other packages
package event
