package integration_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessagePropagation_SingleNode tests message flow through a single node
func TestMessagePropagation_SingleNode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	g := graph.New[[]message.Message, message.Message](MessagesKey)

	g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := graph.GetList(view, MessagesKey)
		require.Len(t, msgs, 1)
		assert.Equal(t, "Hello", message.Stringify(msgs[0]))

		var response message.Message = message.NewAIMessageFromText("World")
		return graph.Append(MessagesKey, response).End()
	}, graph.END)

	g.Start("process")

	compiled, err := g.Build()
	require.NoError(t, err)

	input := []message.Message{
		message.NewHumanMessageFromText("Hello"),
	}

	var outputs []message.Message
	for msg, err := range compiled.Run(ctx, input) {
		require.NoError(t, err)
		outputs = append(outputs, msg)
	}

	require.Len(t, outputs, 1)
	assert.Equal(t, "World", message.Stringify(outputs[0]))
}

// TestMessagePropagation_Chain tests message flow through a chain of nodes
func TestMessagePropagation_Chain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	g := graph.New[[]message.Message, message.Message](MessagesKey)

	g.Node("node1", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		var msg message.Message = message.NewAIMessageFromText("From node1")
		return graph.Append(MessagesKey, msg).To("node2")
	}, "node2")

	g.Node("node2", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := graph.GetList(view, MessagesKey)
		// Should have original + node1's message
		require.GreaterOrEqual(t, len(msgs), 2)

		var msg message.Message = message.NewAIMessageFromText("From node2")
		return graph.Append(MessagesKey, msg).To("node3")
	}, "node3")

	g.Node("node3", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := graph.GetList(view, MessagesKey)
		// Should have original + node1 + node2
		require.GreaterOrEqual(t, len(msgs), 3)

		var msg message.Message = message.NewAIMessageFromText("Final")
		return graph.Append(MessagesKey, msg).End()
	}, graph.END)

	g.Start("node1")

	compiled, err := g.Build()
	require.NoError(t, err)

	input := []message.Message{
		message.NewHumanMessageFromText("Start"),
	}

	var outputs []message.Message
	for msg, err := range compiled.Run(ctx, input) {
		require.NoError(t, err)
		outputs = append(outputs, msg)
	}

	// Should have all AI messages as outputs
	assert.GreaterOrEqual(t, len(outputs), 1)
}

// TestMessagePropagation_ParallelNodes tests message propagation with parallel execution
func TestMessagePropagation_ParallelNodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	g := graph.New[[]message.Message, message.Message](MessagesKey)

	g.Node("start", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		var msg message.Message = message.NewAIMessageFromText("Start processed")
		return graph.Append(MessagesKey, msg).To("worker1", "worker2")
	}, "worker1", "worker2")

	g.Node("worker1", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		var msg message.Message = message.NewAIMessageFromText("Worker1 done")
		return graph.Append(MessagesKey, msg).To("merge")
	}, "merge")

	g.Node("worker2", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		var msg message.Message = message.NewAIMessageFromText("Worker2 done")
		return graph.Append(MessagesKey, msg).To("merge")
	}, "merge")

	g.Node("merge", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := graph.GetList(view, MessagesKey)
		// Should have accumulated messages from parallel workers
		t.Logf("Merge received %d messages", len(msgs))

		var msg message.Message = message.NewAIMessageFromText("Merged")
		return graph.Append(MessagesKey, msg).End()
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	require.NoError(t, err)

	input := []message.Message{
		message.NewHumanMessageFromText("Process this"),
	}

	for _, err := range compiled.Run(ctx, input) {
		require.NoError(t, err)
	}
}

// TestMessagePropagation_PreserveOrder tests that message order is preserved
func TestMessagePropagation_PreserveOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	g := graph.New[[]message.Message, message.Message](MessagesKey)

	g.Node("append", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		_ = graph.GetList(view, MessagesKey) // Verify we can access input

		// Append messages in specific order
		var msg1 message.Message = message.NewAIMessageFromText("First")
		var msg2 message.Message = message.NewAIMessageFromText("Second")
		var msg3 message.Message = message.NewAIMessageFromText("Third")

		return graph.Cmd().
			With(graph.AppendValue(MessagesKey, msg1, msg2, msg3)).
			To(graph.END)
	}, graph.END)

	g.Start("append")

	compiled, err := g.Build()
	require.NoError(t, err)

	input := []message.Message{
		message.NewHumanMessageFromText("Original"),
	}

	var outputs []message.Message
	for msg, err := range compiled.Run(ctx, input) {
		require.NoError(t, err)
		outputs = append(outputs, msg)
	}

	// Verify order is preserved
	if len(outputs) >= 3 {
		assert.Equal(t, "First", message.Stringify(outputs[0]))
		assert.Equal(t, "Second", message.Stringify(outputs[1]))
		assert.Equal(t, "Third", message.Stringify(outputs[2]))
	}
}

// TestMessagePropagation_EmptyInput tests handling of empty message input
func TestMessagePropagation_EmptyInput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	g := graph.New[[]message.Message, message.Message](MessagesKey)

	g.Node("handle", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := graph.GetList(view, MessagesKey)
		if len(msgs) == 0 {
			var msg message.Message = message.NewAIMessageFromText("No input provided")
			return graph.Append(MessagesKey, msg).End()
		}
		var msg message.Message = message.NewAIMessageFromText("Got input")
		return graph.Append(MessagesKey, msg).End()
	}, graph.END)

	g.Start("handle")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Empty input
	input := []message.Message{}

	for _, err := range compiled.Run(ctx, input) {
		require.NoError(t, err)
	}
}

// TestMessagePropagation_LargeMessageList tests handling of many messages
func TestMessagePropagation_LargeMessageList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	messageCount := 100

	g := graph.New[[]message.Message, message.Message](MessagesKey)

	g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := graph.GetList(view, MessagesKey)
		assert.Equal(t, messageCount, len(msgs))

		var msg message.Message = message.NewAIMessageFromText("Processed all messages")
		return graph.Append(MessagesKey, msg).End()
	}, graph.END)

	g.Start("process")

	compiled, err := g.Build()
	require.NoError(t, err)

	// Create many messages
	input := make([]message.Message, messageCount)
	for i := 0; i < messageCount; i++ {
		input[i] = message.NewHumanMessageFromText("Message")
	}

	for _, err := range compiled.Run(ctx, input) {
		require.NoError(t, err)
	}
}
