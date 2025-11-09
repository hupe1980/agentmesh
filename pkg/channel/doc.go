/*
Package channel provides typed communication abstractions for graph execution.

Channels replace direct shared-state access with structured data flow patterns,
enabling distributed execution and deterministic replay. Each channel type
implements specific update semantics:

  - TopicChannel: Accumulates values (append-only, optional max size)
  - LastValueChannel: Stores most recent value (overwrite semantics)
  - BinaryOpChannel: Combines values using custom operator (merge semantics)

Example usage:

	// Create channels
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

	// Read from channels
	msgs, _ := messages.Read(ctx)
	contextVal, _ := context.Read(ctx)

	// Write to channels
	messages.Write(ctx, message.New("Hello"))
	context.Write(ctx, map[string]any{"user": "alice"})
	scores.Write(ctx, 0.95)

Channels are thread-safe and support consistent snapshots for parallel execution.
*/
package channel
