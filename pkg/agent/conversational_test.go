package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/memory"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConversational_Validation(t *testing.T) {
	t.Run("returns error for nil wrapped agent", func(t *testing.T) {
		mem := testutil.NewMockMemory()
		_, err := NewConversational(nil, mem)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wrapped agent")
	})

	t.Run("returns error for nil memory", func(t *testing.T) {
		// Create a simple wrapped agent
		mockModel := &testutil.MockModel{}
		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		_, err = NewConversational(wrappedAgent, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "memory")
	})

	t.Run("creates agent with valid parameters", func(t *testing.T) {
		mockModel := &testutil.MockModel{}
		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)
		assert.NotNil(t, chatAgent)
	})
}

func TestNewConversational_Options(t *testing.T) {
	t.Run("applies WithShortTermMessages", func(t *testing.T) {
		mockModel := &testutil.MockModel{}
		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem,
			WithShortTermMessages(3),
		)
		require.NoError(t, err)
		assert.NotNil(t, chatAgent)
	})

	t.Run("applies WithLongTermMessages", func(t *testing.T) {
		mockModel := &testutil.MockModel{}
		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem,
			WithLongTermMessages(5),
		)
		require.NoError(t, err)
		assert.NotNil(t, chatAgent)
	})

	t.Run("applies WithMinSimilarityScore", func(t *testing.T) {
		mockModel := &testutil.MockModel{}
		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem,
			WithMinSimilarityScore(0.8),
		)
		require.NoError(t, err)
		assert.NotNil(t, chatAgent)
	})

	t.Run("ignores invalid option values", func(t *testing.T) {
		mockModel := &testutil.MockModel{}
		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem,
			WithShortTermMessages(-1),   // Invalid, should be ignored
			WithLongTermMessages(-1),    // Invalid, should be ignored
			WithMinSimilarityScore(1.5), // Invalid, should be ignored
		)
		require.NoError(t, err)
		assert.NotNil(t, chatAgent)
	})
}

func TestConversational_RunWithoutMemory(t *testing.T) {
	t.Run("runs wrapped agent when memory is empty", func(t *testing.T) {
		ctx := context.Background()

		// Create mock model that returns a simple response
		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("Hello! How can I help you?"), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		// Run with a simple message
		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}

		var lastMsg message.Message
		for msg, err := range chatAgent.Run(ctx, messages,
			graph.WithInitialValue(SessionIDKey, "test-session"),
		) {
			require.NoError(t, err)
			lastMsg = msg
		}

		require.NotNil(t, lastMsg)
		assert.Contains(t, message.Stringify(lastMsg), "Hello")
	})
}

func TestConversational_StoresConversation(t *testing.T) {
	t.Run("stores user message and AI response in memory", func(t *testing.T) {
		ctx := context.Background()

		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("I'm doing well, thanks!"), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		messages := []message.Message{
			message.NewHumanMessageFromText("How are you?"),
		}

		// Consume all messages
		for msg, err := range chatAgent.Run(ctx, messages,
			graph.WithInitialValue(SessionIDKey, "test-session"),
		) {
			require.NoError(t, err)
			_ = msg
		}

		// Check that exactly 2 messages were stored (user + AI)
		storedMsgs := mem.GetStoredMessages("test-session")
		require.Len(t, storedMsgs, 2, "should store exactly user message and AI response")

		// Verify the content of stored messages
		assert.Equal(t, message.TypeHuman, storedMsgs[0].Type(), "first stored message should be human")
		assert.Contains(t, message.Stringify(storedMsgs[0]), "How are you?", "should store user message content")

		assert.Equal(t, message.TypeAI, storedMsgs[1].Type(), "second stored message should be AI")
		assert.Contains(t, message.Stringify(storedMsgs[1]), "I'm doing well, thanks!", "should store AI response content")
	})
}

func TestConversational_RecallsFromMemory(t *testing.T) {
	t.Run("recalls previous context from memory", func(t *testing.T) {
		ctx := context.Background()

		var receivedMessages []message.Message
		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				receivedMessages = msgs
				return message.NewAIMessageFromText("Response with context"), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		// Pre-populate memory with previous conversation
		mem := testutil.NewMockMemory()
		previousMsgs := []message.Message{
			message.NewHumanMessageFromText("My name is Alice"),
			message.NewAIMessageFromText("Hello Alice!"),
		}
		err = mem.Store(ctx, "test-session", previousMsgs)
		require.NoError(t, err)

		chatAgent, err := NewConversational(wrappedAgent, mem,
			WithShortTermMessages(10),
		)
		require.NoError(t, err)

		messages := []message.Message{
			message.NewHumanMessageFromText("What is my name?"),
		}

		// Consume all messages
		for msg, err := range chatAgent.Run(ctx, messages,
			graph.WithInitialValue(SessionIDKey, "test-session"),
		) {
			require.NoError(t, err)
			_ = msg
		}

		// Verify that memory context was included in messages to the model
		// The model should receive: system prompt + memory context + current message
		require.GreaterOrEqual(t, len(receivedMessages), 3)

		// Check that the recalled messages are present
		allText := ""
		for _, msg := range receivedMessages {
			allText += message.Stringify(msg) + " "
		}
		assert.Contains(t, allText, "My name is Alice", "should include recalled user message")
		assert.Contains(t, allText, "Hello Alice", "should include recalled AI response")
		assert.Contains(t, allText, "What is my name", "should include current message")
	})
}

func TestConversational_WithRAGAgent(t *testing.T) {
	t.Run("wraps RAG agent with conversational memory", func(t *testing.T) {
		ctx := context.Background()

		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("Based on the documents, AgentMesh is a Go framework."), nil
			}),
		}

		mockRetriever := &mockRetriever{
			docs: []retrieval.Document{
				{PageContent: "AgentMesh is a Go framework for AI agents."},
			},
		}

		ragAgent, err := NewRAG(mockModel, mockRetriever)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatRAG, err := NewConversational(ragAgent, mem)
		require.NoError(t, err)

		messages := []message.Message{
			message.NewHumanMessageFromText("What is AgentMesh?"),
		}

		var lastMsg message.Message
		for msg, err := range chatRAG.Run(ctx, messages,
			graph.WithInitialValue(SessionIDKey, "rag-session"),
		) {
			require.NoError(t, err)
			lastMsg = msg
		}

		require.NotNil(t, lastMsg)
		assert.Contains(t, message.Stringify(lastMsg), "AgentMesh")

		// Verify conversation was stored
		storedMsgs := mem.GetStoredMessages("rag-session")
		assert.GreaterOrEqual(t, len(storedMsgs), 1)
	})
}

func TestConversational_MemoryRecallError(t *testing.T) {
	t.Run("continues execution when memory recall fails", func(t *testing.T) {
		ctx := context.Background()

		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("Response despite memory error"), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		mem.RecallFunc = func(ctx context.Context, sessionID string, filter memory.RecallFilter) ([]message.Message, error) {
			return nil, errors.New("memory recall failed")
		}

		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}

		var lastMsg message.Message
		for msg, err := range chatAgent.Run(ctx, messages,
			graph.WithInitialValue(SessionIDKey, "test-session"),
		) {
			require.NoError(t, err)
			lastMsg = msg
		}

		// Should still get a response despite memory error
		require.NotNil(t, lastMsg)
	})
}

func TestConversational_MemoryStoreError(t *testing.T) {
	t.Run("completes execution when memory store fails", func(t *testing.T) {
		ctx := context.Background()

		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("Response"), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		mem.StoreFunc = func(ctx context.Context, sessionID string, messages []message.Message) error {
			return errors.New("memory store failed")
		}

		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}

		var lastMsg message.Message
		for msg, err := range chatAgent.Run(ctx, messages,
			graph.WithInitialValue(SessionIDKey, "test-session"),
		) {
			require.NoError(t, err)
			lastMsg = msg
		}

		// Should still get a response despite memory store error
		require.NotNil(t, lastMsg)
	})
}

func TestConversational_WrappedAgentError(t *testing.T) {
	t.Run("propagates error from wrapped agent", func(t *testing.T) {
		ctx := context.Background()

		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				return nil, errors.New("model generation failed")
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}

		var gotError error
		for _, err := range chatAgent.Run(ctx, messages,
			graph.WithInitialValue(SessionIDKey, "test-session"),
		) {
			if err != nil {
				gotError = err
				break
			}
		}

		require.Error(t, gotError)
		assert.Contains(t, gotError.Error(), "failed")
	})
}

func TestConversational_EmptyMessages(t *testing.T) {
	t.Run("handles empty message list", func(t *testing.T) {
		ctx := context.Background()

		mockModel := &testutil.MockModel{}
		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		messages := []message.Message{}

		var gotError error
		for _, err := range chatAgent.Run(ctx, messages,
			graph.WithInitialValue(SessionIDKey, "test-session"),
		) {
			if err != nil {
				gotError = err
				break
			}
		}

		require.Error(t, gotError)
		assert.Contains(t, gotError.Error(), "no messages")
	})
}

func TestConversational_MissingSessionID(t *testing.T) {
	t.Run("returns error when session ID not provided", func(t *testing.T) {
		ctx := context.Background()

		mockModel := &testutil.MockModel{}
		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}

		var gotError error
		for _, err := range chatAgent.Run(ctx, messages) {
			if err != nil {
				gotError = err
				break
			}
		}

		require.Error(t, gotError)
		assert.Contains(t, gotError.Error(), "session_id is required")
	})
}

func TestConversational_MultipleTurns(t *testing.T) {
	t.Run("stores messages from each turn and accumulates in memory", func(t *testing.T) {
		ctx := context.Background()

		turnCount := 0
		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				turnCount++
				return message.NewAIMessageFromText("Response " + string(rune('0'+turnCount))), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		// First turn
		messages1 := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}
		for msg, err := range chatAgent.Run(ctx, messages1,
			graph.WithInitialValue(SessionIDKey, "multi-turn-session"),
		) {
			require.NoError(t, err)
			_ = msg
		}

		// After Turn 1: should have exactly 2 messages stored
		storedAfterTurn1 := mem.GetStoredMessages("multi-turn-session")
		require.Len(t, storedAfterTurn1, 2, "after turn 1: should store user + AI")
		assert.Contains(t, message.Stringify(storedAfterTurn1[0]), "Hello")
		assert.Contains(t, message.Stringify(storedAfterTurn1[1]), "Response 1")

		// Second turn
		messages2 := []message.Message{
			message.NewHumanMessageFromText("How are you?"),
		}
		for msg, err := range chatAgent.Run(ctx, messages2,
			graph.WithInitialValue(SessionIDKey, "multi-turn-session"),
		) {
			require.NoError(t, err)
			_ = msg
		}

		// After Turn 2: should have exactly 4 messages stored
		storedAfterTurn2 := mem.GetStoredMessages("multi-turn-session")
		require.Len(t, storedAfterTurn2, 4, "after turn 2: should have 4 messages total")
		assert.Contains(t, message.Stringify(storedAfterTurn2[2]), "How are you?")
		assert.Contains(t, message.Stringify(storedAfterTurn2[3]), "Response 2")

		// Third turn
		messages3 := []message.Message{
			message.NewHumanMessageFromText("Goodbye"),
		}
		for msg, err := range chatAgent.Run(ctx, messages3,
			graph.WithInitialValue(SessionIDKey, "multi-turn-session"),
		) {
			require.NoError(t, err)
			_ = msg
		}

		// After Turn 3: should have exactly 6 messages stored
		storedAfterTurn3 := mem.GetStoredMessages("multi-turn-session")
		require.Len(t, storedAfterTurn3, 6, "after turn 3: should have 6 messages total")
		assert.Contains(t, message.Stringify(storedAfterTurn3[4]), "Goodbye")
		assert.Contains(t, message.Stringify(storedAfterTurn3[5]), "Response 3")

		assert.Equal(t, 3, turnCount, "should have processed 3 turns")
	})

	t.Run("recalls previous turn messages in subsequent turns", func(t *testing.T) {
		ctx := context.Background()

		var turn2ReceivedMessages []message.Message
		turnCount := 0
		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				turnCount++
				if turnCount == 2 {
					turn2ReceivedMessages = msgs
				}
				return message.NewAIMessageFromText("Response " + string(rune('0'+turnCount))), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		mem := testutil.NewMockMemory()
		chatAgent, err := NewConversational(wrappedAgent, mem,
			WithShortTermMessages(10),
		)
		require.NoError(t, err)

		// First turn: introduce context
		for msg, err := range chatAgent.Run(ctx, []message.Message{
			message.NewHumanMessageFromText("My name is Bob and I live in Tokyo"),
		}, graph.WithInitialValue(SessionIDKey, "recall-test")) {
			require.NoError(t, err)
			_ = msg
		}

		// Second turn: should recall first turn
		for msg, err := range chatAgent.Run(ctx, []message.Message{
			message.NewHumanMessageFromText("What is my name?"),
		}, graph.WithInitialValue(SessionIDKey, "recall-test")) {
			require.NoError(t, err)
			_ = msg
		}

		// Verify turn 2 received the context from turn 1
		require.NotNil(t, turn2ReceivedMessages, "should have captured turn 2 messages")

		// Check that turn 1 context is present in turn 2 input
		allText := ""
		for _, msg := range turn2ReceivedMessages {
			allText += message.Stringify(msg) + " "
		}
		assert.Contains(t, allText, "My name is Bob", "turn 2 should receive turn 1 user message")
		assert.Contains(t, allText, "Tokyo", "turn 2 should receive turn 1 location")
		assert.Contains(t, allText, "What is my name", "turn 2 should receive current message")
	})
}

func TestNewConversational_WithInitialSessionID(t *testing.T) {
	t.Run("uses session ID from WithInitialValue", func(t *testing.T) {
		ctx := context.Background()
		mem := testutil.NewMockMemory()
		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("Response"), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		// Run with a runtime session ID
		messages := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}
		for msg, err := range chatAgent.Run(ctx, messages,
			graph.WithInitialValue(SessionIDKey, "runtime-session-123"),
		) {
			require.NoError(t, err)
			_ = msg
		}

		// Should use runtime session ID and store exactly 2 messages
		storedInRuntime := mem.GetStoredMessages("runtime-session-123")
		require.Len(t, storedInRuntime, 2, "should store exactly user + AI in runtime session")
		assert.Contains(t, message.Stringify(storedInRuntime[0]), "Hello")
		assert.Contains(t, message.Stringify(storedInRuntime[1]), "Response")
	})
}

func TestConversational_StoreCalledWithCorrectMessages(t *testing.T) {
	t.Run("store receives user input and AI response", func(t *testing.T) {
		ctx := context.Background()

		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				return message.NewAIMessageFromText("AI says hello back"), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		// Track what Store was called with
		var storedCalls [][]message.Message
		mem := testutil.NewMockMemory()
		mem.StoreFunc = func(ctx context.Context, sessionID string, messages []message.Message) error {
			storedCalls = append(storedCalls, messages)
			return nil
		}

		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		for msg, err := range chatAgent.Run(ctx, []message.Message{
			message.NewHumanMessageFromText("User says hello"),
		}, graph.WithInitialValue(SessionIDKey, "store-test")) {
			require.NoError(t, err)
			_ = msg
		}

		// Verify Store was called exactly once with exactly 2 messages
		require.Len(t, storedCalls, 1, "Store should be called exactly once per turn")
		require.Len(t, storedCalls[0], 2, "Store should receive exactly 2 messages (user + AI)")

		// Verify message types and content
		assert.Equal(t, message.TypeHuman, storedCalls[0][0].Type())
		assert.Contains(t, message.Stringify(storedCalls[0][0]), "User says hello")

		assert.Equal(t, message.TypeAI, storedCalls[0][1].Type())
		assert.Contains(t, message.Stringify(storedCalls[0][1]), "AI says hello back")
	})

	t.Run("store is called after each turn in multi-turn conversation", func(t *testing.T) {
		ctx := context.Background()

		turnCount := 0
		mockModel := &testutil.MockModel{
			GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, msgs []message.Message) (message.Message, error) {
				turnCount++
				return message.NewAIMessageFromText("Response " + string(rune('A'-1+turnCount))), nil
			}),
		}

		wrappedAgent, err := NewReAct(mockModel)
		require.NoError(t, err)

		// Track Store calls - just track, don't call original (which would recurse)
		var storedCalls [][]message.Message
		mem := testutil.NewMockMemory()
		mem.StoreFunc = func(ctx context.Context, sessionID string, messages []message.Message) error {
			storedCalls = append(storedCalls, messages)
			return nil // Just track, don't recurse
		}

		chatAgent, err := NewConversational(wrappedAgent, mem)
		require.NoError(t, err)

		// Turn 1
		for msg, err := range chatAgent.Run(ctx, []message.Message{
			message.NewHumanMessageFromText("First message"),
		}, graph.WithInitialValue(SessionIDKey, "multi-store-test")) {
			require.NoError(t, err)
			_ = msg
		}

		require.Len(t, storedCalls, 1, "should have 1 store call after turn 1")
		assert.Contains(t, message.Stringify(storedCalls[0][0]), "First message")

		// Turn 2
		for msg, err := range chatAgent.Run(ctx, []message.Message{
			message.NewHumanMessageFromText("Second message"),
		}, graph.WithInitialValue(SessionIDKey, "multi-store-test")) {
			require.NoError(t, err)
			_ = msg
		}

		require.Len(t, storedCalls, 2, "should have 2 store calls after turn 2")
		assert.Contains(t, message.Stringify(storedCalls[1][0]), "Second message")

		// Turn 3
		for msg, err := range chatAgent.Run(ctx, []message.Message{
			message.NewHumanMessageFromText("Third message"),
		}, graph.WithInitialValue(SessionIDKey, "multi-store-test")) {
			require.NoError(t, err)
			_ = msg
		}

		require.Len(t, storedCalls, 3, "should have 3 store calls after turn 3")
		assert.Contains(t, message.Stringify(storedCalls[2][0]), "Third message")
	})
}
