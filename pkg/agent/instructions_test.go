package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createInstructionsTestScope creates a test scope for instructions tests.
func createInstructionsTestScope(data map[string]any) message.Scope {
	return testutil.NewTestScopeFromMap[message.Message](data)
}

func TestNewInstructions(t *testing.T) {
	t.Parallel()

	t.Run("creates static instructions", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("You are a helpful assistant")
		assert.True(t, instructions.IsStatic())
		assert.Nil(t, instructions.provider)
		assert.NotNil(t, instructions.template)
	})

	t.Run("creates instructions with template placeholders", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("You are helping {{.userName}}")
		assert.True(t, instructions.IsStatic())
	})
}

func TestNewInstructionsFromProvider(t *testing.T) {
	t.Parallel()

	t.Run("creates dynamic instructions from provider", func(t *testing.T) {
		t.Parallel()

		provider := InstructionsProviderFunc(func(ctx context.Context, scope message.Scope) (string, error) {
			return "dynamic instructions", nil
		})

		instructions := NewInstructionsFromProvider(provider)
		assert.False(t, instructions.IsStatic())
		assert.NotNil(t, instructions.provider)
		assert.Nil(t, instructions.template)
	})
}

func TestNewInstructionsFromFunc(t *testing.T) {
	t.Parallel()

	t.Run("creates dynamic instructions from function", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructionsFromFunc(func(ctx context.Context, scope message.Scope) (string, error) {
			return "function-based instructions", nil
		})

		assert.False(t, instructions.IsStatic())
		assert.NotNil(t, instructions.provider)
	})
}

func TestInstructions_IsStatic(t *testing.T) {
	t.Parallel()

	t.Run("returns true for template-based instructions", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("static text")
		assert.True(t, instructions.IsStatic())
	})

	t.Run("returns false for provider-based instructions", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructionsFromFunc(func(ctx context.Context, scope message.Scope) (string, error) {
			return "dynamic", nil
		})
		assert.False(t, instructions.IsStatic())
	})
}

func TestInstructions_Resolve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("resolves static instructions without placeholders", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("You are a helpful assistant")
		scope := createInstructionsTestScope(nil)

		result, err := instructions.Resolve(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "You are a helpful assistant", result)
	})

	t.Run("resolves template with placeholders from scope", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("Hello {{.userName}}, your task is {{.task}}")
		scope := createInstructionsTestScope(map[string]any{
			"userName": "Alice",
			"task":     "coding",
		})

		result, err := instructions.Resolve(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "Hello Alice, your task is coding", result)
	})

	t.Run("resolves template with default function", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("Task: {{default \"general\" .task}}")
		scope := createInstructionsTestScope(nil)

		result, err := instructions.Resolve(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "Task: general", result)
	})

	t.Run("resolves template with upper function", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("Name: {{.name | upper}}")
		scope := createInstructionsTestScope(map[string]any{
			"name": "alice",
		})

		result, err := instructions.Resolve(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "Name: ALICE", result)
	})

	t.Run("resolves template with lower function", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("Name: {{.name | lower}}")
		scope := createInstructionsTestScope(map[string]any{
			"name": "ALICE",
		})

		result, err := instructions.Resolve(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "Name: alice", result)
	})

	t.Run("resolves dynamic instructions from provider", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructionsFromFunc(func(ctx context.Context, scope message.Scope) (string, error) {
			name, _ := scope.GetValue("userName")
			return "Dynamic instructions for " + name.(string), nil
		})

		scope := createInstructionsTestScope(map[string]any{
			"userName": "Bob",
		})

		result, err := instructions.Resolve(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "Dynamic instructions for Bob", result)
	})

	t.Run("returns error from provider", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("provider error")
		instructions := NewInstructionsFromFunc(func(ctx context.Context, scope message.Scope) (string, error) {
			return "", expectedErr
		})

		scope := createInstructionsTestScope(nil)
		_, err := instructions.Resolve(ctx, scope)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("returns empty string for nil template", func(t *testing.T) {
		t.Parallel()

		instructions := Instructions{} // zero value
		scope := createInstructionsTestScope(nil)

		result, err := instructions.Resolve(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("resolves template with conditional", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("{{if .verbose}}Detailed mode enabled. {{end}}Process the request.")
		scope := createInstructionsTestScope(map[string]any{
			"verbose": true,
		})

		result, err := instructions.Resolve(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "Detailed mode enabled. Process the request.", result)
	})

	t.Run("resolves template with conditional false", func(t *testing.T) {
		t.Parallel()

		instructions := NewInstructions("{{if .verbose}}Detailed mode enabled. {{end}}Process the request.")
		scope := createInstructionsTestScope(map[string]any{
			"verbose": false,
		})

		result, err := instructions.Resolve(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "Process the request.", result)
	})
}

func TestInstructionsProviderFunc(t *testing.T) {
	t.Parallel()

	t.Run("implements InstructionsProvider interface", func(t *testing.T) {
		t.Parallel()

		var provider InstructionsProvider = InstructionsProviderFunc(func(ctx context.Context, scope message.Scope) (string, error) {
			return "test", nil
		})

		ctx := context.Background()
		scope := createInstructionsTestScope(nil)

		result, err := provider.Instructions(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "test", result)
	})

	t.Run("receives context and scope", func(t *testing.T) {
		t.Parallel()

		type ctxKey string
		key := ctxKey("testKey")

		provider := InstructionsProviderFunc(func(ctx context.Context, scope message.Scope) (string, error) {
			ctxVal := ctx.Value(key).(string)
			scopeVal, _ := scope.GetValue("scopeKey")
			return ctxVal + "-" + scopeVal.(string), nil
		})

		ctx := context.WithValue(context.Background(), key, "ctxValue")
		scope := createInstructionsTestScope(map[string]any{
			"scopeKey": "scopeValue",
		})

		result, err := provider.Instructions(ctx, scope)
		require.NoError(t, err)
		assert.Equal(t, "ctxValue-scopeValue", result)
	})
}
