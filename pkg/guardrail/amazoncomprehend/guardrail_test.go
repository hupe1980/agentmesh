package amazoncomprehend

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/stretchr/testify/assert"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
)

// MockClient is a mock implementation of the Client interface.
type MockClient struct {
	DetectSentimentFunc   func(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error)
	DetectPiiEntitiesFunc func(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error)
}

func (m *MockClient) DetectSentiment(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
	if m.DetectSentimentFunc != nil {
		return m.DetectSentimentFunc(ctx, params, optFns...)
	}
	return nil, errors.New("DetectSentimentFunc not set")
}

func (m *MockClient) DetectPiiEntities(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
	if m.DetectPiiEntitiesFunc != nil {
		return m.DetectPiiEntitiesFunc(ctx, params, optFns...)
	}
	return nil, errors.New("DetectPiiEntitiesFunc not set")
}

// Ensure MockClient implements Client interface
var _ Client = (*MockClient)(nil)

func TestSentimentGuardrail_Name(t *testing.T) {
	client := &MockClient{}
	g := NewSentiment(client)
	assert.Equal(t, "comprehend-sentiment", g.Name())

	g2 := NewSentiment(client, WithSentimentName("custom-name"))
	assert.Equal(t, "custom-name", g2.Name())
}

func TestSentimentGuardrail_Check_AllowsPositiveSentiment(t *testing.T) {
	client := &MockClient{
		DetectSentimentFunc: func(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
			return &comprehend.DetectSentimentOutput{
				Sentiment: types.SentimentTypePositive,
				SentimentScore: &types.SentimentScore{
					Positive: aws.Float32(0.9),
					Negative: aws.Float32(0.05),
					Neutral:  aws.Float32(0.03),
					Mixed:    aws.Float32(0.02),
				},
			}, nil
		},
	}

	g := NewSentiment(client)
	result, err := g.Check(context.Background(), "I love this product!")

	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionAllow, result.Action)
}

func TestSentimentGuardrail_Check_RejectsNegativeSentiment(t *testing.T) {
	client := &MockClient{
		DetectSentimentFunc: func(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
			return &comprehend.DetectSentimentOutput{
				Sentiment: types.SentimentTypeNegative,
				SentimentScore: &types.SentimentScore{
					Positive: aws.Float32(0.02),
					Negative: aws.Float32(0.92),
					Neutral:  aws.Float32(0.04),
					Mixed:    aws.Float32(0.02),
				},
			}, nil
		},
	}

	g := NewSentiment(client)
	result, err := g.Check(context.Background(), "I hate this product!")

	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionReject, result.Action)
	assert.Contains(t, result.Message, "negative sentiment detected")
}

func TestSentimentGuardrail_Check_RaisesWhenConfigured(t *testing.T) {
	client := &MockClient{
		DetectSentimentFunc: func(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
			return &comprehend.DetectSentimentOutput{
				Sentiment: types.SentimentTypeNegative,
				SentimentScore: &types.SentimentScore{
					Positive: aws.Float32(0.02),
					Negative: aws.Float32(0.92),
					Neutral:  aws.Float32(0.04),
					Mixed:    aws.Float32(0.02),
				},
			}, nil
		},
	}

	g := NewSentiment(client, WithSentimentAction(guardrail.ActionRaise))
	result, err := g.Check(context.Background(), "I hate this!")

	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionRaise, result.Action)
}

func TestSentimentGuardrail_Check_CustomThreshold(t *testing.T) {
	client := &MockClient{
		DetectSentimentFunc: func(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
			return &comprehend.DetectSentimentOutput{
				Sentiment: types.SentimentTypeNegative,
				SentimentScore: &types.SentimentScore{
					Positive: aws.Float32(0.1),
					Negative: aws.Float32(0.6), // Below default 0.8, but above custom 0.5
					Neutral:  aws.Float32(0.2),
					Mixed:    aws.Float32(0.1),
				},
			}, nil
		},
	}

	// With default threshold (0.8), should allow
	g1 := NewSentiment(client)
	result1, err := g1.Check(context.Background(), "test")
	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionAllow, result1.Action)

	// With custom threshold (0.5), should reject
	g2 := NewSentiment(client, WithBlockNegative(0.5))
	result2, err := g2.Check(context.Background(), "test")
	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionReject, result2.Action)
}

func TestSentimentGuardrail_Check_APIError(t *testing.T) {
	client := &MockClient{
		DetectSentimentFunc: func(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
			return nil, errors.New("API error")
		},
	}

	g := NewSentiment(client)
	_, err := g.Check(context.Background(), "test")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "comprehend sentiment error")
}

func TestSentimentGuardrail_LanguageCode(t *testing.T) {
	var capturedInput *comprehend.DetectSentimentInput

	client := &MockClient{
		DetectSentimentFunc: func(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
			capturedInput = params
			return &comprehend.DetectSentimentOutput{
				Sentiment: types.SentimentTypeNeutral,
				SentimentScore: &types.SentimentScore{
					Positive: aws.Float32(0.3),
					Negative: aws.Float32(0.2),
					Neutral:  aws.Float32(0.4),
					Mixed:    aws.Float32(0.1),
				},
			}, nil
		},
	}

	g := NewSentiment(client, WithLanguageCode(types.LanguageCodeEs))
	_, err := g.Check(context.Background(), "Hola mundo")

	assert.NoError(t, err)
	assert.Equal(t, types.LanguageCodeEs, capturedInput.LanguageCode)
}

func TestPIIGuardrail_Name(t *testing.T) {
	client := &MockClient{}
	g := NewPII(client)
	assert.Equal(t, "comprehend-pii", g.Name())

	g2 := NewPII(client, WithPIIName("custom-pii"))
	assert.Equal(t, "custom-pii", g2.Name())
}

func TestPIIGuardrail_Check_AllowsNoPII(t *testing.T) {
	client := &MockClient{
		DetectPiiEntitiesFunc: func(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
			return &comprehend.DetectPiiEntitiesOutput{
				Entities: []types.PiiEntity{},
			}, nil
		},
	}

	g := NewPII(client)
	result, err := g.Check(context.Background(), "Hello, how are you?")

	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionAllow, result.Action)
}

func TestPIIGuardrail_Check_RejectsDetectedPII(t *testing.T) {
	client := &MockClient{
		DetectPiiEntitiesFunc: func(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
			return &comprehend.DetectPiiEntitiesOutput{
				Entities: []types.PiiEntity{
					{
						Type:        types.PiiEntityTypeEmail,
						Score:       aws.Float32(0.99),
						BeginOffset: aws.Int32(0),
						EndOffset:   aws.Int32(15),
					},
				},
			}, nil
		},
	}

	g := NewPII(client)
	result, err := g.Check(context.Background(), "test@example.com")

	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionReject, result.Action)
	assert.Contains(t, result.Message, "PII detected")
	assert.Contains(t, result.Message, "EMAIL")
}

func TestPIIGuardrail_Check_RaisesWhenConfigured(t *testing.T) {
	client := &MockClient{
		DetectPiiEntitiesFunc: func(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
			return &comprehend.DetectPiiEntitiesOutput{
				Entities: []types.PiiEntity{
					{Type: types.PiiEntityTypePhone, Score: aws.Float32(0.95)},
				},
			}, nil
		},
	}

	g := NewPII(client, WithPIIAction(guardrail.ActionRaise))
	result, err := g.Check(context.Background(), "Call me at 555-1234")

	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionRaise, result.Action)
}

func TestPIIGuardrail_Check_BlockedTypes(t *testing.T) {
	client := &MockClient{
		DetectPiiEntitiesFunc: func(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
			return &comprehend.DetectPiiEntitiesOutput{
				Entities: []types.PiiEntity{
					{Type: types.PiiEntityTypeEmail, Score: aws.Float32(0.99)},
					{Type: types.PiiEntityTypeName, Score: aws.Float32(0.9)},
				},
			}, nil
		},
	}

	// Only block EMAIL, not NAME
	g := NewPII(client, WithBlockedPIITypes(types.PiiEntityTypeEmail))
	result, err := g.Check(context.Background(), "John's email is john@example.com")

	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionReject, result.Action)
	assert.Contains(t, result.Message, "EMAIL")
}

func TestPIIGuardrail_Check_AllowsIfBlockedTypesNotDetected(t *testing.T) {
	client := &MockClient{
		DetectPiiEntitiesFunc: func(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
			return &comprehend.DetectPiiEntitiesOutput{
				Entities: []types.PiiEntity{
					{Type: types.PiiEntityTypeName, Score: aws.Float32(0.9)},
				},
			}, nil
		},
	}

	// Only block SSN (not detected), NAME is detected but not blocked
	g := NewPII(client, WithBlockedPIITypes(types.PiiEntityTypeSsn))
	result, err := g.Check(context.Background(), "My name is John")

	assert.NoError(t, err)
	assert.Equal(t, guardrail.ActionAllow, result.Action)
}

func TestPIIGuardrail_Check_APIError(t *testing.T) {
	client := &MockClient{
		DetectPiiEntitiesFunc: func(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
			return nil, errors.New("API error")
		},
	}

	g := NewPII(client)
	_, err := g.Check(context.Background(), "test")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "comprehend pii detection error")
}

func TestPIIGuardrail_LanguageCode(t *testing.T) {
	var capturedInput *comprehend.DetectPiiEntitiesInput

	client := &MockClient{
		DetectPiiEntitiesFunc: func(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
			capturedInput = params
			return &comprehend.DetectPiiEntitiesOutput{
				Entities: []types.PiiEntity{},
			}, nil
		},
	}

	g := NewPII(client, WithPIILanguageCode(types.LanguageCodeDe))
	_, err := g.Check(context.Background(), "Hallo Welt")

	assert.NoError(t, err)
	assert.Equal(t, types.LanguageCodeDe, capturedInput.LanguageCode)
}

func TestGuardrails_ImplementInterface(t *testing.T) {
	client := &MockClient{}

	var sentimentGuardrail guardrail.Guardrail[string] = NewSentiment(client)
	var piiGuardrail guardrail.Guardrail[string] = NewPII(client)

	assert.NotNil(t, sentimentGuardrail)
	assert.NotNil(t, piiGuardrail)
}
