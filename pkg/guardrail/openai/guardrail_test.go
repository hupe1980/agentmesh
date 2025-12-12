package openai

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/openai/openai-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockClient is a mock implementation of the Client interface for testing.
type MockClient struct {
	ModerateFunc func(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error)
}

// Moderate calls the mock function.
func (m *MockClient) Moderate(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
	if m.ModerateFunc != nil {
		return m.ModerateFunc(ctx, params)
	}
	return &openai.ModerationNewResponse{}, nil
}

func TestGuardrail_Name(t *testing.T) {
	client := &MockClient{}
	g := New(client)
	assert.Equal(t, "openai-moderation", g.Name())

	g2 := New(client, WithName("custom-name"))
	assert.Equal(t, "custom-name", g2.Name())
}

func TestGuardrail_Check_AllowsSafeContent(t *testing.T) {
	client := &MockClient{
		ModerateFunc: func(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
			return &openai.ModerationNewResponse{
				Results: []openai.Moderation{
					{
						Flagged: false,
						Categories: openai.ModerationCategories{
							Hate:       false,
							Violence:   false,
							Harassment: false,
						},
						CategoryScores: openai.ModerationCategoryScores{
							Hate:       0.001,
							Violence:   0.002,
							Harassment: 0.001,
						},
					},
				},
			}, nil
		},
	}

	g := New(client)
	result, err := g.Check(context.Background(), "Hello, how are you?")

	require.NoError(t, err)
	assert.True(t, result.IsAllowed())
	assert.Equal(t, guardrail.ActionAllow, result.Action)
}

func TestGuardrail_Check_RejectsFlaggedContent(t *testing.T) {
	client := &MockClient{
		ModerateFunc: func(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
			return &openai.ModerationNewResponse{
				Results: []openai.Moderation{
					{
						Flagged: true,
						Categories: openai.ModerationCategories{
							Hate:       true,
							Violence:   false,
							Harassment: true,
						},
						CategoryScores: openai.ModerationCategoryScores{
							Hate:       0.95,
							Violence:   0.01,
							Harassment: 0.88,
						},
					},
				},
			}, nil
		},
	}

	g := New(client)
	result, err := g.Check(context.Background(), "hateful content")

	require.NoError(t, err)
	assert.False(t, result.IsAllowed())
	assert.Equal(t, guardrail.ActionReject, result.Action)
	assert.Contains(t, result.Message, "content flagged for")
	assert.NotNil(t, result.Info)
}

func TestGuardrail_Check_RaisesWhenConfigured(t *testing.T) {
	client := &MockClient{
		ModerateFunc: func(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
			return &openai.ModerationNewResponse{
				Results: []openai.Moderation{
					{
						Flagged: true,
						Categories: openai.ModerationCategories{
							Violence: true,
						},
						CategoryScores: openai.ModerationCategoryScores{
							Violence: 0.99,
						},
					},
				},
			}, nil
		},
	}

	g := New(client, WithAction(guardrail.ActionRaise))
	result, err := g.Check(context.Background(), "violent content")

	require.NoError(t, err)
	assert.True(t, result.IsTripwire())
	assert.Equal(t, guardrail.ActionRaise, result.Action)
}

func TestGuardrail_Check_CustomThreshold(t *testing.T) {
	client := &MockClient{
		ModerateFunc: func(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
			return &openai.ModerationNewResponse{
				Results: []openai.Moderation{
					{
						Flagged: false, // Not flagged by OpenAI's default
						Categories: openai.ModerationCategories{
							Hate: false,
						},
						CategoryScores: openai.ModerationCategoryScores{
							Hate: 0.45, // Below OpenAI's threshold but above our custom threshold
						},
					},
				},
			}, nil
		},
	}

	// With custom threshold of 0.3, this should be flagged
	g := New(client, WithThreshold(CategoryHate, 0.3))
	result, err := g.Check(context.Background(), "borderline content")

	require.NoError(t, err)
	assert.False(t, result.IsAllowed())
	assert.Equal(t, guardrail.ActionReject, result.Action)
}

func TestGuardrail_Check_EmptyResults(t *testing.T) {
	client := &MockClient{
		ModerateFunc: func(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
			return &openai.ModerationNewResponse{
				Results: []openai.Moderation{},
			}, nil
		},
	}

	g := New(client)
	result, err := g.Check(context.Background(), "some content")

	require.NoError(t, err)
	assert.True(t, result.IsAllowed())
}

func TestGuardrail_Check_APIError(t *testing.T) {
	client := &MockClient{
		ModerateFunc: func(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
			return nil, errors.New("API rate limit exceeded")
		},
	}

	g := New(client)
	result, err := g.Check(context.Background(), "some content")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "openai moderation api error")
}

func TestGuardrail_Options(t *testing.T) {
	client := &MockClient{}

	t.Run("default options", func(t *testing.T) {
		g := New(client)
		assert.Equal(t, "openai-moderation", g.name)
		assert.Equal(t, guardrail.ActionReject, g.action)
		assert.Equal(t, openai.ModerationModelOmniModerationLatest, g.model)
	})

	t.Run("custom name", func(t *testing.T) {
		g := New(client, WithName("my-guardrail"))
		assert.Equal(t, "my-guardrail", g.name)
	})

	t.Run("custom action", func(t *testing.T) {
		g := New(client, WithAction(guardrail.ActionRaise))
		assert.Equal(t, guardrail.ActionRaise, g.action)
	})

	t.Run("custom model", func(t *testing.T) {
		g := New(client, WithModel(openai.ModerationModelTextModerationLatest))
		assert.Equal(t, openai.ModerationModelTextModerationLatest, g.model)
	})

	t.Run("custom thresholds", func(t *testing.T) {
		thresholds := map[Category]float64{
			CategoryHate:     0.5,
			CategoryViolence: 0.7,
		}
		g := New(client, WithThresholds(thresholds))
		assert.Equal(t, 0.5, g.thresholds[CategoryHate])
		assert.Equal(t, 0.7, g.thresholds[CategoryViolence])
	})

	t.Run("single threshold", func(t *testing.T) {
		g := New(client, WithThreshold(CategorySexual, 0.3))
		assert.Equal(t, 0.3, g.thresholds[CategorySexual])
	})
}

func TestGuardrail_ImplementsInterface(t *testing.T) {
	client := &MockClient{}
	g := New(client)

	// Verify the guardrail implements the interface
	var _ guardrail.Guardrail[string] = g
}
