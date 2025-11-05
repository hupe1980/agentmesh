package memory

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/require"
)

func TestVectorMemory_StoreAndRecall(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := NewSimpleEmbedder(64)
	mem := NewVectorMemory(embedder)

	messages := []message.Message{
		message.NewHumanMessageFromText("What is the price of Product A?"),
		message.NewAIMessageFromText("Product A costs $100"),
		message.NewHumanMessageFromText("How about Product B?"),
		message.NewAIMessageFromText("Product B costs $200"),
	}

	// Store messages
	err := mem.Store(ctx, "session-1", messages)
	require.NoError(t, err)

	// Recall all messages
	recalled, err := mem.Recall(ctx, "session-1", RecallFilter{K: 10})
	require.NoError(t, err)
	require.Len(t, recalled, 4)
}

func TestVectorMemory_SemanticSearch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := NewSimpleEmbedder(128)
	mem := NewVectorMemory(embedder)

	messages := []message.Message{
		message.NewHumanMessageFromText("What is the price of Product A?"),
		message.NewAIMessageFromText("Product A costs $100"),
		message.NewHumanMessageFromText("Tell me about weather forecast"),
		message.NewAIMessageFromText("It will be sunny tomorrow"),
	}

	err := mem.Store(ctx, "session-1", messages)
	require.NoError(t, err)

	// Search for pricing-related messages
	recalled, err := mem.Recall(ctx, "session-1", RecallFilter{
		Query: "product pricing cost",
		K:     2,
	})
	require.NoError(t, err)
	require.Len(t, recalled, 2)
}

func TestVectorMemory_MinScore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := NewSimpleEmbedder(64)
	mem := NewVectorMemory(embedder)

	messages := []message.Message{
		message.NewHumanMessageFromText("Hello"),
		message.NewAIMessageFromText("Hi there!"),
	}

	err := mem.Store(ctx, "session-1", messages)
	require.NoError(t, err)

	// High threshold should return fewer results
	recalled, err := mem.Recall(ctx, "session-1", RecallFilter{
		Query:    "greeting",
		K:        10,
		MinScore: 0.9, // Very high threshold
	})
	require.NoError(t, err)
	// Should return 0-2 messages depending on embedding similarity
	require.LessOrEqual(t, len(recalled), 2)
}

func TestVectorMemory_TypeFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := NewSimpleEmbedder(64)
	mem := NewVectorMemory(embedder)

	messages := []message.Message{
		message.NewHumanMessageFromText("Question 1"),
		message.NewAIMessageFromText("Answer 1"),
		message.NewHumanMessageFromText("Question 2"),
		message.NewAIMessageFromText("Answer 2"),
	}

	err := mem.Store(ctx, "session-1", messages)
	require.NoError(t, err)

	// Recall only AI messages
	recalled, err := mem.Recall(ctx, "session-1", RecallFilter{
		Types: []message.Type{message.TypeAI},
		K:     10,
	})
	require.NoError(t, err)
	require.Len(t, recalled, 2)
	for _, msg := range recalled {
		require.Equal(t, message.TypeAI, msg.Type())
	}
}

func TestVectorMemory_TimeFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := NewSimpleEmbedder(64)
	mem := NewVectorMemory(embedder)

	// Store first batch
	messages1 := []message.Message{
		message.NewHumanMessageFromText("Old message"),
	}
	err := mem.Store(ctx, "session-1", messages1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)

	// Store second batch
	messages2 := []message.Message{
		message.NewHumanMessageFromText("New message"),
	}
	err = mem.Store(ctx, "session-1", messages2)
	require.NoError(t, err)

	// Recall only messages after cutoff
	recalled, err := mem.Recall(ctx, "session-1", RecallFilter{
		After: &cutoff,
		K:     10,
	})
	require.NoError(t, err)
	require.Len(t, recalled, 1)
}

func TestVectorMemory_Clear(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := NewSimpleEmbedder(64)
	mem := NewVectorMemory(embedder)

	messages := []message.Message{
		message.NewHumanMessageFromText("Test message"),
	}

	err := mem.Store(ctx, "session-1", messages)
	require.NoError(t, err)

	// Clear session
	err = mem.Clear(ctx, "session-1")
	require.NoError(t, err)

	// Should return no messages
	recalled, err := mem.Recall(ctx, "session-1", RecallFilter{K: 10})
	require.NoError(t, err)
	require.Empty(t, recalled)
}

func TestVectorMemory_MultipleSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := NewSimpleEmbedder(64)
	mem := NewVectorMemory(embedder)

	// Store in session 1
	err := mem.Store(ctx, "session-1", []message.Message{
		message.NewHumanMessageFromText("Session 1 message"),
	})
	require.NoError(t, err)

	// Store in session 2
	err = mem.Store(ctx, "session-2", []message.Message{
		message.NewHumanMessageFromText("Session 2 message"),
	})
	require.NoError(t, err)

	// Recall from session 1
	recalled1, err := mem.Recall(ctx, "session-1", RecallFilter{K: 10})
	require.NoError(t, err)
	require.Len(t, recalled1, 1)

	// Recall from session 2
	recalled2, err := mem.Recall(ctx, "session-2", RecallFilter{K: 10})
	require.NoError(t, err)
	require.Len(t, recalled2, 1)

	// Get all sessions
	sessions, err := mem.Sessions(ctx)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
}

func TestSimpleMemory_StoreAndRecall(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mem := NewSimpleMemory(100)

	messages := []message.Message{
		message.NewHumanMessageFromText("Message 1"),
		message.NewAIMessageFromText("Response 1"),
	}

	err := mem.Store(ctx, "session-1", messages)
	require.NoError(t, err)

	recalled, err := mem.Recall(ctx, "session-1", RecallFilter{K: 10})
	require.NoError(t, err)
	require.Len(t, recalled, 2)
}

func TestSimpleMemory_MaxSize(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mem := NewSimpleMemory(2) // Limit to 2 messages

	// Store 4 messages
	messages := []message.Message{
		message.NewHumanMessageFromText("Message 1"),
		message.NewAIMessageFromText("Message 2"),
		message.NewHumanMessageFromText("Message 3"),
		message.NewAIMessageFromText("Message 4"),
	}

	err := mem.Store(ctx, "session-1", messages)
	require.NoError(t, err)

	// Should only retain last 2
	recalled, err := mem.Recall(ctx, "session-1", RecallFilter{K: 10})
	require.NoError(t, err)
	require.Len(t, recalled, 2)
}

func TestSimpleMemory_MostRecentFirst(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mem := NewSimpleMemory(0) // Unlimited

	messages := []message.Message{
		message.NewHumanMessageFromText("First"),
		message.NewHumanMessageFromText("Second"),
		message.NewHumanMessageFromText("Third"),
	}

	err := mem.Store(ctx, "session-1", messages)
	require.NoError(t, err)

	recalled, err := mem.Recall(ctx, "session-1", RecallFilter{K: 2})
	require.NoError(t, err)
	require.Len(t, recalled, 2)

	// Should get most recent first
	parts := recalled[0].Parts()
	require.Len(t, parts, 1)
	textPart := parts[0].(message.TextPart)
	require.Equal(t, "Third", textPart.Text)
}

func TestSimpleEmbedder_Dimensions(t *testing.T) {
	t.Parallel()

	embedder := NewSimpleEmbedder(256)
	require.Equal(t, 256, embedder.Dimensions())
}

func TestSimpleEmbedder_Embed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := NewSimpleEmbedder(128)

	embedding, err := embedder.Embed(ctx, "test text")
	require.NoError(t, err)
	require.Len(t, embedding, 128)

	// Embeddings should be deterministic
	embedding2, err := embedder.Embed(ctx, "test text")
	require.NoError(t, err)
	require.Equal(t, embedding, embedding2)
}

func TestSimpleEmbedder_EmbedBatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := NewSimpleEmbedder(64)

	texts := []string{"text 1", "text 2", "text 3"}
	embeddings, err := embedder.EmbedBatch(ctx, texts)
	require.NoError(t, err)
	require.Len(t, embeddings, 3)
	for _, emb := range embeddings {
		require.Len(t, emb, 64)
	}
}

func TestCosineSimilarity(t *testing.T) {
	t.Parallel()

	// Identical vectors should have similarity of 1.0
	a := []float64{1, 0, 0}
	b := []float64{1, 0, 0}
	sim := cosineSimilarity(a, b)
	require.InDelta(t, 1.0, sim, 0.001)

	// Orthogonal vectors should have similarity of 0.0
	c := []float64{1, 0, 0}
	d := []float64{0, 1, 0}
	sim2 := cosineSimilarity(c, d)
	require.InDelta(t, 0.0, sim2, 0.001)

	// Opposite vectors should have similarity of -1.0
	e := []float64{1, 0, 0}
	f := []float64{-1, 0, 0}
	sim3 := cosineSimilarity(e, f)
	require.InDelta(t, -1.0, sim3, 0.001)
}

func TestRecallFilter_Normalize(t *testing.T) {
	t.Parallel()

	// Test default K
	filter := RecallFilter{}
	filter.Normalize()
	require.Equal(t, DefaultK, filter.K)

	// Test negative min score
	filter2 := RecallFilter{MinScore: -0.5}
	filter2.Normalize()
	require.Equal(t, 0.0, filter2.MinScore)

	// Test min score > 1
	filter3 := RecallFilter{MinScore: 1.5}
	filter3.Normalize()
	require.Equal(t, 1.0, filter3.MinScore)
}
