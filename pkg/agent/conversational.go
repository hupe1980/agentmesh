package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/memory"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// SessionIDKey is the state key for the current session identifier.
var SessionIDKey = graph.NewKey("session_id", "")

// MemoryContextKey is the state key for messages retrieved from memory.
var MemoryContextKey = graph.NewKey("memory_context", []message.Message{})

// conversationalOptions holds configuration for Conversational agents.
type conversationalOptions struct {
	// maxRecallMessages limits how many messages to recall from memory
	maxRecallMessages int
	// minSimilarityScore for semantic search in memory
	minSimilarityScore float64
	// failOnStoreError causes the agent to fail if memory storage fails
	failOnStoreError bool
}

func defaultConversationalOptions() conversationalOptions {
	return conversationalOptions{
		maxRecallMessages:  10,
		minSimilarityScore: 0.7,
	}
}

// ConversationalOption configures a Conversational agent.
type ConversationalOption func(*conversationalOptions)

// WithMaxRecallMessages sets the maximum number of messages to recall from memory.
func WithMaxRecallMessages(n int) ConversationalOption {
	return func(c *conversationalOptions) {
		if n > 0 {
			c.maxRecallMessages = n
		}
	}
}

// WithMinSimilarityScore sets the minimum similarity score for memory search.
func WithMinSimilarityScore(score float64) ConversationalOption {
	return func(c *conversationalOptions) {
		if score >= 0 && score <= 1 {
			c.minSimilarityScore = score
		}
	}
}

// WithFailOnStoreError causes the agent to return an error if memory storage fails.
// By default, storage errors are silently ignored since memory is non-critical.
func WithFailOnStoreError(fail bool) ConversationalOption {
	return func(c *conversationalOptions) {
		c.failOnStoreError = fail
	}
}

// NewConversational creates a memory-enhanced conversational agent that:
//  1. Recalls relevant context from memory before the agent runs
//  2. Executes the wrapped agent (ReAct, RAG, etc.) as a subgraph
//  3. Stores the conversation exchange in memory after completion
//
// The wrapped agent can be any *message.Graph (ReAct, RAG, Reflection, etc.).
// This enables composable, memory-aware conversational experiences.
//
// A session ID must be provided at runtime using [graph.WithInitialValue].
//
// Returns a *message.Graph for type-safe composition.
//
// Example:
//
//	// Create a ReAct agent
//	reactAgent, _ := agent.NewReAct(model, agent.WithTools(tools))
//
//	// Wrap it with memory
//	mem := memory.NewSimple() // or semantic memory
//	chatAgent, _ := agent.NewConversational(reactAgent, mem)
//
//	// Use it with a session ID
//	for msg, err := range chatAgent.Run(ctx, messages,
//	    graph.WithInitialValue(agent.SessionIDKey, "user-123"),
//	) {
//	    // handle msg
//	}
func NewConversational(
	wrappedAgent *message.Graph,
	mem memory.Memory,
	opts ...ConversationalOption,
) (*message.Graph, error) {
	if err := validate.All(
		validate.NotNil(wrappedAgent, "wrapped agent"),
		validate.NotNil(mem, "memory"),
	); err != nil {
		return nil, err
	}

	config := defaultConversationalOptions()
	for _, opt := range opts {
		opt(&config)
	}

	return buildConversationalGraph(wrappedAgent, mem, config)
}

// buildConversationalGraph constructs the graph structure:
// START -> memory_recall -> agent -> memory_store -> END
func buildConversationalGraph(
	wrappedAgent *message.Graph,
	mem memory.Memory,
	config conversationalOptions,
) (*message.Graph, error) {
	b := message.NewGraphBuilder(SessionIDKey, MemoryContextKey)

	// Node 1: Recall relevant context from memory
	b.Node("memory_recall", createMemoryRecallNode(mem, config), "agent")

	// Node 2: Run the wrapped agent as a subgraph
	b.Node("agent", createConversationalAgentNode(wrappedAgent), "memory_store")

	// Node 3: Store the conversation in memory
	b.Node("memory_store", createMemoryStoreNode(mem, config), graph.END)

	b.Start("memory_recall")

	return b.Build()
}

// createMemoryRecallNode creates a node that recalls relevant context from memory
// and prepends it to the conversation.
func createMemoryRecallNode(mem memory.Memory, config conversationalOptions) graph.NodeFunc {
	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := GetMessages(view)
		if len(msgs) == 0 {
			return graph.Fail(fmt.Errorf("agent/conversational: no messages"))
		}

		// Get session ID from state (must be provided via WithInitialValue)
		sessionID := graph.Get(view, SessionIDKey)
		if sessionID == "" {
			return graph.Fail(fmt.Errorf("agent/conversational: session_id is required, use graph.WithInitialValue(agent.SessionIDKey, \"your-session-id\")"))
		}

		// Extract user query for semantic search
		query, err := extractUserQuery(msgs)
		if err != nil {
			// No user query found, skip memory recall
			return graph.Set(MemoryContextKey, []message.Message{}).
				With(graph.SetValue(SessionIDKey, sessionID)).
				To("agent")
		}

		// Recall relevant messages from memory
		recalledMsgs, err := mem.Recall(ctx, sessionID, memory.RecallFilter{
			Query:    query,
			K:        config.maxRecallMessages,
			MinScore: config.minSimilarityScore,
		})
		if err != nil {
			// Log but continue without memory context
			recalledMsgs = []message.Message{}
		}

		// Store recalled context and session ID in state
		return graph.Set(MemoryContextKey, recalledMsgs).
			With(graph.SetValue(SessionIDKey, sessionID)).
			To("agent")
	}
}

// createConversationalAgentNode creates a node that executes the wrapped agent
// as a subgraph, prepending memory context to the messages.
func createConversationalAgentNode(wrappedAgent *message.Graph) graph.NodeFunc {
	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := GetMessages(view)
		if len(msgs) == 0 {
			return graph.Fail(fmt.Errorf("agent/conversational: no messages"))
		}

		// Get memory context and prepend to messages
		memoryContext := graph.Get(view, MemoryContextKey)
		if len(memoryContext) > 0 {
			// Prepend memory context before current messages
			msgs = append(memoryContext, msgs...)
		}

		// Run wrapped agent as subgraph - gets the last emitted message
		lastMsg, err := graph.Last(wrappedAgent.Run(ctx, msgs))
		if err != nil {
			return graph.Fail(fmt.Errorf("agent/conversational: agent failed: %w", err))
		}

		return graph.Append(MessagesKey, lastMsg).To("memory_store")
	}
}

// createMemoryStoreNode creates a node that stores the conversation in memory.
func createMemoryStoreNode(mem memory.Memory, config conversationalOptions) graph.NodeFunc {
	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := GetMessages(view)
		if len(msgs) < 2 {
			// Need at least user message + AI response
			return graph.To(graph.END)
		}

		sessionID := graph.Get(view, SessionIDKey)
		if sessionID == "" {
			// Session ID should have been validated in memory_recall
			return graph.To(graph.END)
		}

		// Get the last exchange (user query + AI response)
		// We store only the current exchange, not the memory context
		memoryContext := graph.Get(view, MemoryContextKey)
		startIdx := len(memoryContext) // Skip prepended memory messages

		if startIdx >= len(msgs) {
			return graph.To(graph.END)
		}

		// Store messages from this conversation turn
		toStore := msgs[startIdx:]
		if len(toStore) > 0 {
			if err := mem.Store(ctx, sessionID, toStore); err != nil {
				if config.failOnStoreError {
					return graph.Fail(fmt.Errorf("agent/conversational: memory store failed: %w", err))
				}
				logging.FromContext(ctx).Warn("memory store failed", "error", err)
			}
		}

		return graph.To(graph.END)
	}
}
