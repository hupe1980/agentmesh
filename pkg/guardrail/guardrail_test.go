package guardrail

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAction_String(t *testing.T) {
	tests := []struct {
		action   Action
		expected string
	}{
		{ActionAllow, "allow"},
		{ActionReject, "reject"},
		{ActionRaise, "raise"},
		{Action(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.action.String())
		})
	}
}

func TestResult_Constructors(t *testing.T) {
	t.Run("Allow", func(t *testing.T) {
		result := Allow()
		assert.Equal(t, ActionAllow, result.Action)
		assert.Empty(t, result.Message)
		assert.True(t, result.IsAllowed())
		assert.False(t, result.IsRejection())
		assert.False(t, result.IsTripwire())
	})

	t.Run("AllowWithInfo", func(t *testing.T) {
		info := map[string]any{"key": "value"}
		result := AllowWithInfo(info)
		assert.Equal(t, ActionAllow, result.Action)
		assert.Equal(t, info, result.Info)
		assert.True(t, result.IsAllowed())
	})

	t.Run("Reject", func(t *testing.T) {
		result := Reject("blocked content")
		assert.Equal(t, ActionReject, result.Action)
		assert.Equal(t, "blocked content", result.Message)
		assert.False(t, result.IsAllowed())
		assert.True(t, result.IsRejection())
		assert.False(t, result.IsTripwire())
	})

	t.Run("RejectWithInfo", func(t *testing.T) {
		info := map[string]any{"reason": "test"}
		result := RejectWithInfo("rejected", info)
		assert.Equal(t, ActionReject, result.Action)
		assert.Equal(t, "rejected", result.Message)
		assert.Equal(t, info, result.Info)
		assert.True(t, result.IsRejection())
	})

	t.Run("Raise", func(t *testing.T) {
		result := Raise("critical error")
		assert.Equal(t, ActionRaise, result.Action)
		assert.Equal(t, "critical error", result.Message)
		assert.False(t, result.IsAllowed())
		assert.False(t, result.IsRejection())
		assert.True(t, result.IsTripwire())
	})

	t.Run("RaiseWithInfo", func(t *testing.T) {
		info := map[string]any{"severity": "high"}
		result := RaiseWithInfo("raised", info)
		assert.Equal(t, ActionRaise, result.Action)
		assert.Equal(t, "raised", result.Message)
		assert.Equal(t, info, result.Info)
		assert.True(t, result.IsTripwire())
	})
}

func TestTripwireError(t *testing.T) {
	result := RaiseWithInfo("test message", map[string]any{"key": "value"})
	err := NewTripwireError("test-guardrail", result)

	assert.Equal(t, "test-guardrail", err.GuardrailName)
	assert.Equal(t, "test message", err.Message)
	assert.Equal(t, map[string]any{"key": "value"}, err.Info)
	assert.Contains(t, err.Error(), "test-guardrail")
	assert.Contains(t, err.Error(), "test message")
}

func TestRejection(t *testing.T) {
	result := RejectWithInfo("rejection message", map[string]any{"reason": "test"})
	rejection := NewRejection("test-guardrail", result)

	assert.Equal(t, "test-guardrail", rejection.GuardrailName)
	assert.Equal(t, "rejection message", rejection.Message)
	assert.Equal(t, map[string]any{"reason": "test"}, rejection.Info)
	assert.Contains(t, rejection.Error(), "test-guardrail")
	assert.Contains(t, rejection.Error(), "rejection message")
}

// MockGuardrail is a mock implementation for testing
type MockGuardrail struct {
	name   string
	result *Result
	err    error
}

func (m *MockGuardrail) Name() string { return m.name }
func (m *MockGuardrail) Check(_ context.Context, _ string) (*Result, error) {
	return m.result, m.err
}

var _ Guardrail[string] = (*MockGuardrail)(nil)

func TestChain(t *testing.T) {
	ctx := context.Background()

	t.Run("AllAllow", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", result: Allow()}
		g2 := &MockGuardrail{name: "g2", result: Allow()}
		g3 := &MockGuardrail{name: "g3", result: Allow()}

		result, err := Chain(ctx, "test", g1, g2, g3)
		assert.NoError(t, err)
		assert.Equal(t, ActionAllow, result.Action)
	})

	t.Run("StopsAtFirstReject", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", result: Allow()}
		g2 := &MockGuardrail{name: "g2", result: Reject("blocked")}
		g3 := &MockGuardrail{name: "g3", result: Allow()} // Should not be reached

		result, err := Chain(ctx, "test", g1, g2, g3)
		assert.NoError(t, err)
		assert.Equal(t, ActionReject, result.Action)
		assert.Equal(t, "blocked", result.Message)
	})

	t.Run("StopsAtFirstRaise", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", result: Allow()}
		g2 := &MockGuardrail{name: "g2", result: Raise("critical")}
		g3 := &MockGuardrail{name: "g3", result: Allow()}

		result, err := Chain(ctx, "test", g1, g2, g3)
		assert.NoError(t, err)
		assert.Equal(t, ActionRaise, result.Action)
		assert.Equal(t, "critical", result.Message)
	})

	t.Run("ReturnsError", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", result: Allow()}
		g2 := &MockGuardrail{name: "g2", err: errors.New("api error")}

		_, err := Chain(ctx, "test", g1, g2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "api error")
	})

	t.Run("EmptyChain", func(t *testing.T) {
		result, err := Chain[string](ctx, "test")
		assert.NoError(t, err)
		assert.Equal(t, ActionAllow, result.Action)
	})
}

func TestChainGuardrail(t *testing.T) {
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		chain := NewChainGuardrail[string]("test-chain")
		assert.Equal(t, "test-chain", chain.Name())
	})

	t.Run("Check", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", result: Allow()}
		g2 := &MockGuardrail{name: "g2", result: Reject("blocked")}

		chain := NewChainGuardrail("test-chain", g1, g2)
		result, err := chain.Check(ctx, "test")

		assert.NoError(t, err)
		assert.Equal(t, ActionReject, result.Action)
	})

	t.Run("All", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", result: Allow()}
		chain := All(g1)
		assert.Equal(t, "all", chain.Name())
	})
}

func TestAnyGuardrail(t *testing.T) {
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		anyG := NewAnyGuardrail[string]("test-any")
		assert.Equal(t, "test-any", anyG.Name())
	})

	t.Run("AllowsIfAnyAllows", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", result: Reject("rejected1")}
		g2 := &MockGuardrail{name: "g2", result: Allow()}
		g3 := &MockGuardrail{name: "g3", result: Reject("rejected3")}

		anyG := NewAnyGuardrail("test-any", g1, g2, g3)
		result, err := anyG.Check(ctx, "test")

		assert.NoError(t, err)
		assert.Equal(t, ActionAllow, result.Action)
	})

	t.Run("RejectsIfNoneAllow", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", result: Reject("rejected1")}
		g2 := &MockGuardrail{name: "g2", result: Reject("rejected2")}

		anyG := NewAnyGuardrail("test-any", g1, g2)
		result, err := anyG.Check(ctx, "test")

		assert.NoError(t, err)
		assert.Equal(t, ActionReject, result.Action)
		assert.Equal(t, "rejected2", result.Message) // Last rejection
	})

	t.Run("EmptyAny", func(t *testing.T) {
		anyG := NewAnyGuardrail[string]("test-any")
		result, err := anyG.Check(ctx, "test")

		assert.NoError(t, err)
		assert.Equal(t, ActionAllow, result.Action)
	})

	t.Run("ReturnsError", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", err: errors.New("error")}

		anyG := NewAnyGuardrail("test-any", g1)
		_, err := anyG.Check(ctx, "test")

		assert.Error(t, err)
	})

	t.Run("Any", func(t *testing.T) {
		g1 := &MockGuardrail{name: "g1", result: Allow()}
		anyG := Any(g1)
		assert.Equal(t, "any", anyG.Name())
	})
}
