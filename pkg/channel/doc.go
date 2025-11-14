/*
Package channel provides typed communication abstractions for graph execution.

Channels replace direct shared-state access with structured data flow patterns,
enabling distributed execution and deterministic replay. Each channel type
implements specific update semantics:

  - TopicChannel: Accumulates values (append-only, optional max size)
  - LastValueChannel: Stores most recent value (overwrite semantics)
  - BinaryOpChannel: Combines values using custom operator (merge semantics)

# Interface Hierarchy

The package uses three interfaces to separate concerns:

1. Channel (user-facing) - Safe operations for graph nodes:
  - Name() - Channel identifier
  - Read(ctx) - Read current value
  - Write(ctx, value) - Write with channel-specific semantics

2. VersionedChannel (internal) - Runtime operations for the graph engine:
  - Version() - Cache invalidation tracking
  - Snapshot(ctx) - Point-in-time state capture
  - Clone() - Deep copy for checkpointing

3. ResettableChannel (admin) - Dangerous operations requiring explicit control:
  - Reset(ctx) - Clear state (WARNING: only use between graph runs)

All channel implementations (TopicChannel, LastValueChannel, BinaryOpChannel)
satisfy all three interfaces. User code should interact only with the base
Channel interface. The graph runtime uses VersionedChannel internally.

Example usage:

	// Create channels (returns VersionedChannel but use as Channel)
	messages := channel.NewTopicChannel("messages", 100) // Keep last 100
	context := channel.NewLastValueChannel("context")
	scores := channel.NewBinaryOpChannel("scores", 0.0, func(cur, inc any) any {
		return cur.(float64) + inc.(float64) // Sum scores
	})

	// Create channel set
	channels := channel.NewSet()
	channels.Add(messages)
	channels.Add(context)
	channels.Add(scores)

	// Read from channels (user-facing operations)
	msgs, _ := messages.Read(ctx)
	contextVal, _ := context.Read(ctx)

	// Write to channels (user-facing operations)
	messages.Write(ctx, message.New("Hello"))
	context.Write(ctx, map[string]any{"user": "alice"})
	scores.Write(ctx, 0.95)

	// Internal operations (runtime only - type assertion required)
	if vch, ok := messages.(channel.VersionedChannel); ok {
		version := vch.Version()      // Cache tracking
		snap, _ := vch.Snapshot(ctx)  // Consistent read
		clone := vch.Clone()          // Deep copy
	}

	// Admin operations (use with caution)
	if rch, ok := messages.(channel.ResettableChannel); ok {
		rch.Reset(ctx)  // WARNING: Clears all data
	}

Channels are thread-safe and support consistent snapshots for parallel execution.
*/
package channel
