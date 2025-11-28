package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetryPolicyExecution tests that retry policies work correctly during execution.
func TestRetryPolicyExecution(t *testing.T) {
	t.Run("succeeds after retries with Builder API", func(t *testing.T) {
		attemptCount := 0

		// Register messages key on state builder first
		messagesKey := state.NewListKey[message.Message](MessagesKeyName, 0)
		stateBuilder := state.NewManagerBuilder()
		require.NoError(t, state.RegisterListKey(stateBuilder, messagesKey))
		mgr := stateBuilder.Build()

		builder, err := NewBuilder(NewMessagePregelExecutor(), WithManager[[]message.Message, message.Message](mgr))
		require.NoError(t, err)

		targets := []string{EndNode}

		// Node that fails twice, then succeeds
		builder.AddNodeFuncWithRetry("retry_node", targets,
			func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				attemptCount++
				if attemptCount < 3 {
					return nil, nil, errors.New("transient error")
				}
				return []string{EndNode}, state.Updates{
					MessagesKeyName: []message.Message{message.NewAIMessageFromText("success")},
				}, nil
			},
			NewRetryPolicy().
				WithMaxAttempts(5).
				Build(),
		)

		builder.SetEntryPoint("retry_node")
		compiled, err := builder.Compile()
		require.NoError(t, err)

		// Execute the graph
		ctx := context.Background()
		messages := []message.Message{message.NewHumanMessageFromText("test")}
		_, err = Last(compiled.Run(ctx, messages))
		require.NoError(t, err)

		assert.Equal(t, 3, attemptCount, "should have attempted 3 times before succeeding")
	})

	t.Run("fails after max attempts", func(t *testing.T) {
		attemptCount := 0

		// Register messages key on state builder first
		messagesKey := state.NewListKey[message.Message](MessagesKeyName, 0)
		stateBuilder := state.NewManagerBuilder()
		require.NoError(t, state.RegisterListKey(stateBuilder, messagesKey))
		mgr := stateBuilder.Build()

		builder, err := NewBuilder(NewMessagePregelExecutor(), WithManager[[]message.Message, message.Message](mgr))
		require.NoError(t, err)

		targets := []string{EndNode} // Node that always fails
		builder.AddNodeFuncWithRetry("always_fails", targets,
			func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				attemptCount++
				return nil, nil, errors.New("permanent error")
			},
			NewRetryPolicy().
				WithMaxAttempts(3).
				Build(),
		)

		builder.SetEntryPoint("always_fails")
		compiled, err := builder.Compile()
		require.NoError(t, err)

		// Execute and expect error
		ctx := context.Background()
		messages := []message.Message{message.NewHumanMessageFromText("test")}
		_, err = Last(compiled.Run(ctx, messages))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "max retry attempts (3) exceeded")
		assert.Equal(t, 3, attemptCount, "should have attempted exactly 3 times")
	})

	t.Run("selective error retry", func(t *testing.T) {
		ErrTransient := errors.New("transient error")
		ErrPermanent := errors.New("permanent error")

		t.Run("does not retry permanent errors", func(t *testing.T) {
			attemptCount := 0

			stateBuilder := state.NewManagerBuilder()
			messagesKey := state.NewListKey[message.Message](MessagesKeyName, 0)
			require.NoError(t, state.RegisterListKey(stateBuilder, messagesKey))
			mgr := stateBuilder.Build()

			builder, err := NewBuilder(NewMessagePregelExecutor(), WithManager[[]message.Message, message.Message](mgr))
			require.NoError(t, err)

			targets := []string{EndNode}

			builder.AddNodeFuncWithRetry("selective_node", targets,
				func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
					attemptCount++
					return nil, nil, ErrPermanent // Not in retryable list
				},
				NewRetryPolicy().
					WithMaxAttempts(5).
					WithRetryableErrors(ErrTransient). // Only retry transient
					Build(),
			)

			builder.SetEntryPoint("selective_node")
			compiled, err := builder.Compile()
			require.NoError(t, err)

			ctx := context.Background()
			messages := []message.Message{message.NewHumanMessageFromText("test")}
			_, err = Last(compiled.Run(ctx, messages))

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPermanent)
			assert.Equal(t, 1, attemptCount, "should not retry permanent error")
		})

		t.Run("retries transient errors", func(t *testing.T) {
			attemptCount := 0

			stateBuilder := state.NewManagerBuilder()
			messagesKey := state.NewListKey[message.Message](MessagesKeyName, 0)
			require.NoError(t, state.RegisterListKey(stateBuilder, messagesKey))
			mgr := stateBuilder.Build()

			builder, err := NewBuilder(NewMessagePregelExecutor(), WithManager[[]message.Message, message.Message](mgr))
			require.NoError(t, err)

			targets := []string{EndNode}

			builder.AddNodeFuncWithRetry("selective_node", targets,
				func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
					attemptCount++
					if attemptCount < 3 {
						return nil, nil, ErrTransient // Retryable
					}
					return []string{EndNode}, state.Updates{
						MessagesKeyName: []message.Message{message.NewAIMessageFromText("success")},
					}, nil
				},
				NewRetryPolicy().
					WithMaxAttempts(5).
					WithRetryableErrors(ErrTransient).
					Build(),
			)

			builder.SetEntryPoint("selective_node")
			compiled, err := builder.Compile()
			require.NoError(t, err)

			ctx := context.Background()
			messages := []message.Message{message.NewHumanMessageFromText("test")}
			_, err = Last(compiled.Run(ctx, messages))
			require.NoError(t, err)

			assert.Equal(t, 3, attemptCount, "should retry transient error until success")
		})
	})
}

// TestRetryPolicyPriority verifies NodeWithRetry interface takes priority over NodeOption.
func TestRetryPolicyPriority(t *testing.T) {
	t.Run("NodeWithRetry interface verified", func(t *testing.T) {
		stateBuilder := state.NewManagerBuilder()
		messagesKey := state.NewListKey[message.Message](MessagesKeyName, 0)
		require.NoError(t, state.RegisterListKey(stateBuilder, messagesKey))
		mgr := stateBuilder.Build()

		builder, err := NewBuilder(NewMessagePregelExecutor(), WithManager[[]message.Message, message.Message](mgr))
		require.NoError(t, err)

		targets := []string{EndNode}

		// Add node with retry policy via Builder
		builder.AddNodeFuncWithRetry("test_node", targets,
			func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{EndNode}, state.Updates{
					MessagesKeyName: []message.Message{message.NewAIMessageFromText("done")},
				}, nil
			},
			NewRetryPolicy().WithMaxAttempts(5).Build(),
		)

		builder.SetEntryPoint("test_node")
		compiled, err := builder.Compile()
		require.NoError(t, err)

		// Verify the node implements NodeWithRetry interface
		node := compiled.graph.Nodes["test_node"]
		require.NotNil(t, node)

		retryNode, ok := node.(NodeWithRetry)
		require.True(t, ok, "node should implement NodeWithRetry interface")

		policy := retryNode.RetryPolicy()
		require.NotNil(t, policy)
		assert.Equal(t, 5, policy.MaxAttempts, "retry policy should have 5 attempts")
	})
}
