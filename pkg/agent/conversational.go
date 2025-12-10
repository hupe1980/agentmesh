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
	// shortTermMessages is the number of recent messages to always include (recency-based)
	shortTermMessages int
	// longTermMessages is the number of semantically similar messages to recall
	longTermMessages int
	// minSimilarityScore for semantic search in memory
	minSimilarityScore float64
	// failOnStoreError causes the agent to fail if memory storage fails
	failOnStoreError bool
}

func defaultConversationalOptions() conversationalOptions {
	return conversationalOptions{
		shortTermMessages:  5, // Last 5 messages for immediate context
		longTermMessages:   5, // 5 semantically similar messages from history
		minSimilarityScore: 0.5,
	}
}

// ConversationalOption configures a Conversational agent.
type ConversationalOption func(*conversationalOptions)

// WithShortTermMessages sets the number of recent messages to always include.
// These are the last N messages from the conversation, providing immediate context.
// Default is 5.
func WithShortTermMessages(n int) ConversationalOption {
	return func(c *conversationalOptions) {
		if n >= 0 {
			c.shortTermMessages = n
		}
	}
}

// WithLongTermMessages sets the number of semantically similar messages to recall.
// These are retrieved via semantic search from the conversation history.
// Default is 5.
func WithLongTermMessages(n int) ConversationalOption {
	return func(c *conversationalOptions) {
		if n >= 0 {
			c.longTermMessages = n
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
// using both short-term (recent) and long-term (semantic) retrieval.
func createMemoryRecallNode(mem memory.Memory, config conversationalOptions) graph.NodeFunc {
	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		logger := logging.FromContext(ctx)

		msgs := GetMessages(view)
		if len(msgs) == 0 {
			return graph.Fail(ErrNoMessages)
		}

		// Get session ID from state (must be provided via WithInitialValue)
		sessionID := graph.Get(view, SessionIDKey)
		if sessionID == "" {
			return graph.Fail(ErrSessionIDRequired)
		}

		var combinedMsgs []message.Message

		// 1. Short-term memory: Get most recent messages (no semantic search)
		if config.shortTermMessages > 0 {
			recentMsgs, err := mem.Recall(ctx, sessionID, memory.RecallFilter{
				K: config.shortTermMessages,
				// No Query = returns most recent by timestamp (descending)
			})

			switch {
			case err != nil:
				logger.Debug("short-term memory recall failed", "error", err)
			case len(recentMsgs) == 0:
				logger.Debug("short-term memory empty", "session_id", sessionID)
			default:
				logger.Debug("short-term memory recall",
					"count", len(recentMsgs),
					"session_id", sessionID,
				)
				for i, msg := range recentMsgs {
					logger.Debug("short-term message",
						"index", i,
						"type", msg.Type(),
						"content", truncateForLog(msg.String()),
					)
				}
				// Reverse to chronological order (oldest first) for conversation flow
				for i := len(recentMsgs) - 1; i >= 0; i-- {
					combinedMsgs = append(combinedMsgs, recentMsgs[i])
				}
			}
		}

		// 2. Long-term memory: Semantic search for relevant context
		combinedMsgs = recallLongTermMemory(ctx, logger, mem, sessionID, msgs, config, combinedMsgs)

		logger.Debug("combined memory context",
			"total_messages", len(combinedMsgs),
			"session_id", sessionID,
		)

		// Store recalled context and session ID in state
		return graph.Set(MemoryContextKey, combinedMsgs).
			With(graph.SetValue(SessionIDKey, sessionID)).
			To("agent")
	}
}

// logTruncateLen is the maximum length for log message content.
const logTruncateLen = 100

// truncateForLog truncates a string for logging purposes.
func truncateForLog(s string) string {
	if len(s) <= logTruncateLen {
		return s
	}
	return s[:logTruncateLen] + "..."
}

// recallLongTermMemory performs semantic search for relevant context from long-term memory.
func recallLongTermMemory(
	ctx context.Context,
	logger logging.Logger,
	mem memory.Memory,
	sessionID string,
	msgs []message.Message,
	config conversationalOptions,
	combinedMsgs []message.Message,
) []message.Message {
	if config.longTermMessages <= 0 {
		return combinedMsgs
	}

	query, err := extractUserQuery(msgs)
	if err != nil {
		return combinedMsgs
	}

	logger.Debug("long-term memory query", "query", truncateForLog(query))

	semanticMsgs, err := mem.Recall(ctx, sessionID, memory.RecallFilter{
		Query:    query,
		K:        config.longTermMessages,
		MinScore: config.minSimilarityScore,
	})

	switch {
	case err != nil:
		logger.Debug("long-term memory recall failed", "error", err)
	case len(semanticMsgs) > 0:
		logger.Debug("long-term memory recall",
			"count", len(semanticMsgs),
			"min_score", config.minSimilarityScore,
		)
		for i, msg := range semanticMsgs {
			logger.Debug("long-term message",
				"index", i,
				"type", msg.Type(),
				"content", truncateForLog(msg.String()),
			)
		}
		// Deduplicate: only add messages not already in short-term
		combinedMsgs = deduplicateMessages(combinedMsgs, semanticMsgs)
	}

	return combinedMsgs
}

// deduplicateMessages adds messages from additional to base, skipping duplicates.
func deduplicateMessages(base, additional []message.Message) []message.Message {
	seen := make(map[string]bool)

	// Mark existing messages as seen (using content as key)
	for _, msg := range base {
		key := msg.String()
		seen[key] = true
	}

	// Add non-duplicate messages
	for _, msg := range additional {
		key := msg.String()
		if !seen[key] {
			base = append(base, msg)
			seen[key] = true
		}
	}

	return base
}

// createConversationalAgentNode creates a node that executes the wrapped agent
// as a subgraph, prepending memory context to the messages.
func createConversationalAgentNode(wrappedAgent *message.Graph) graph.NodeFunc {
	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msgs := GetMessages(view)
		if len(msgs) == 0 {
			return graph.Fail(ErrNoMessages)
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
		logger := logging.FromContext(ctx)

		msgs := GetMessages(view)
		if len(msgs) < 2 {
			// Need at least user message + AI response
			logger.Debug("memory store skipped: not enough messages", "count", len(msgs))
			return graph.To(graph.END)
		}

		sessionID := graph.Get(view, SessionIDKey)
		if sessionID == "" {
			// Session ID should have been validated in memory_recall
			logger.Debug("memory store skipped: no session ID")
			return graph.To(graph.END)
		}

		// Store only the current exchange (last 2 messages: user input + AI response)
		// We only want this turn, not messages from previous turns
		toStore := msgs[len(msgs)-2:]

		logger.Debug("storing messages in memory",
			"count", len(toStore),
			"session_id", sessionID,
		)
		for i, msg := range toStore {
			logger.Debug("storing message",
				"index", i,
				"type", msg.Type(),
				"content", truncateForLog(msg.String()),
			)
		}
		if err := mem.Store(ctx, sessionID, toStore); err != nil {
			if config.failOnStoreError {
				return graph.Fail(fmt.Errorf("agent/conversational: memory store failed: %w", err))
			}
			logger.Warn("memory store failed", "error", err)
		}

		return graph.To(graph.END)
	}
}
