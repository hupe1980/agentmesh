// Package amazoncomprehend provides guardrails using AWS Comprehend for sentiment and PII detection.
package amazoncomprehend

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/comprehend/types"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
)

// Client is an interface for the AWS Comprehend client.
// This abstraction allows for easier testing and mocking.
type Client interface {
	DetectSentiment(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error)
	DetectPiiEntities(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error)
}

// Ensure *comprehend.Client implements the Client interface.
var _ Client = (*comprehend.Client)(nil)

// SentimentGuardrail checks content sentiment using AWS Comprehend.
type SentimentGuardrail struct {
	client            Client
	name              string
	action            guardrail.Action
	languageCode      types.LanguageCode
	negativeThreshold float32
}

// SentimentOptions configures the sentiment guardrail.
type SentimentOptions struct {
	Name              string
	Action            guardrail.Action
	LanguageCode      types.LanguageCode
	NegativeThreshold float32
}

// SentimentOption is a function that configures the sentiment guardrail.
type SentimentOption func(*SentimentOptions)

// WithSentimentName sets the guardrail name.
func WithSentimentName(name string) SentimentOption {
	return func(o *SentimentOptions) {
		o.Name = name
	}
}

// WithSentimentAction sets the action for flagged content.
func WithSentimentAction(action guardrail.Action) SentimentOption {
	return func(o *SentimentOptions) {
		o.Action = action
	}
}

// WithLanguageCode sets the language code.
func WithLanguageCode(code types.LanguageCode) SentimentOption {
	return func(o *SentimentOptions) {
		o.LanguageCode = code
	}
}

// WithBlockNegative sets the threshold for blocking negative sentiment.
func WithBlockNegative(threshold float32) SentimentOption {
	return func(o *SentimentOptions) {
		o.NegativeThreshold = threshold
	}
}

// NewSentiment creates a new sentiment guardrail using AWS Comprehend.
func NewSentiment(client Client, opts ...SentimentOption) *SentimentGuardrail {
	options := &SentimentOptions{
		Name:              "comprehend-sentiment",
		Action:            guardrail.ActionReject,
		LanguageCode:      types.LanguageCodeEn,
		NegativeThreshold: 0.8,
	}

	for _, opt := range opts {
		opt(options)
	}

	return &SentimentGuardrail{
		client:            client,
		name:              options.Name,
		action:            options.Action,
		languageCode:      options.LanguageCode,
		negativeThreshold: options.NegativeThreshold,
	}
}

// Name returns the guardrail name.
func (g *SentimentGuardrail) Name() string {
	return g.name
}

// Check analyzes the sentiment of the input text using AWS Comprehend.
//
//nolint:nestif // Nested conditionals for nil-checks on AWS response fields
func (g *SentimentGuardrail) Check(ctx context.Context, input string) (*guardrail.Result, error) {
	result, err := g.client.DetectSentiment(ctx, &comprehend.DetectSentimentInput{
		Text:         aws.String(input),
		LanguageCode: g.languageCode,
	})
	if err != nil {
		return nil, fmt.Errorf("comprehend sentiment error: %w", err)
	}

	if result.SentimentScore != nil && result.SentimentScore.Negative != nil {
		negativeScore := *result.SentimentScore.Negative
		if negativeScore >= g.negativeThreshold {
			message := fmt.Sprintf("negative sentiment detected (score: %.2f)", negativeScore)
			info := map[string]any{
				"sentiment": string(result.Sentiment),
			}
			if result.SentimentScore.Positive != nil {
				info["positive_score"] = float64(*result.SentimentScore.Positive)
			}
			if result.SentimentScore.Negative != nil {
				info["negative_score"] = float64(*result.SentimentScore.Negative)
			}
			if result.SentimentScore.Neutral != nil {
				info["neutral_score"] = float64(*result.SentimentScore.Neutral)
			}
			if result.SentimentScore.Mixed != nil {
				info["mixed_score"] = float64(*result.SentimentScore.Mixed)
			}

			switch g.action {
			case guardrail.ActionRaise:
				return guardrail.RaiseWithInfo(message, info), nil
			default:
				return guardrail.RejectWithInfo(message, info), nil
			}
		}
	}

	return guardrail.Allow(), nil
}

// Ensure SentimentGuardrail implements the interface.
var _ guardrail.Guardrail[string] = (*SentimentGuardrail)(nil)

// PIIGuardrail detects PII in content using AWS Comprehend.
type PIIGuardrail struct {
	client       Client
	name         string
	action       guardrail.Action
	languageCode types.LanguageCode
	blockedTypes map[types.PiiEntityType]bool
}

// PIIOptions configures the PII guardrail.
type PIIOptions struct {
	Name         string
	Action       guardrail.Action
	LanguageCode types.LanguageCode
	BlockedTypes []types.PiiEntityType
}

// PIIOption is a function that configures the PII guardrail.
type PIIOption func(*PIIOptions)

// WithPIIName sets the guardrail name.
func WithPIIName(name string) PIIOption {
	return func(o *PIIOptions) {
		o.Name = name
	}
}

// WithPIIAction sets the action for detected PII.
func WithPIIAction(action guardrail.Action) PIIOption {
	return func(o *PIIOptions) {
		o.Action = action
	}
}

// WithPIILanguageCode sets the language code.
func WithPIILanguageCode(code types.LanguageCode) PIIOption {
	return func(o *PIIOptions) {
		o.LanguageCode = code
	}
}

// WithBlockedPIITypes sets which PII types to block.
func WithBlockedPIITypes(piiTypes ...types.PiiEntityType) PIIOption {
	return func(o *PIIOptions) {
		o.BlockedTypes = piiTypes
	}
}

// NewPII creates a new PII detection guardrail using AWS Comprehend.
func NewPII(client Client, opts ...PIIOption) *PIIGuardrail {
	options := &PIIOptions{
		Name:         "comprehend-pii",
		Action:       guardrail.ActionReject,
		LanguageCode: types.LanguageCodeEn,
		BlockedTypes: nil, // Block all PII types by default
	}

	for _, opt := range opts {
		opt(options)
	}

	blockedTypes := make(map[types.PiiEntityType]bool)
	for _, t := range options.BlockedTypes {
		blockedTypes[t] = true
	}

	return &PIIGuardrail{
		client:       client,
		name:         options.Name,
		action:       options.Action,
		languageCode: options.LanguageCode,
		blockedTypes: blockedTypes,
	}
}

// Name returns the guardrail name.
func (g *PIIGuardrail) Name() string {
	return g.name
}

// Check detects PII in the input text using AWS Comprehend.
func (g *PIIGuardrail) Check(ctx context.Context, input string) (*guardrail.Result, error) {
	result, err := g.client.DetectPiiEntities(ctx, &comprehend.DetectPiiEntitiesInput{
		Text:         aws.String(input),
		LanguageCode: g.languageCode,
	})
	if err != nil {
		return nil, fmt.Errorf("comprehend pii detection error: %w", err)
	}

	var detectedPII []string

	for _, entity := range result.Entities {
		// If no specific types are configured, block all PII
		if len(g.blockedTypes) == 0 || g.blockedTypes[entity.Type] {
			detectedPII = append(detectedPII, string(entity.Type))
		}
	}

	if len(detectedPII) > 0 {
		message := fmt.Sprintf("PII detected: %s", strings.Join(detectedPII, ", "))
		info := map[string]any{
			"pii_types": detectedPII,
			"pii_count": len(detectedPII),
		}

		switch g.action {
		case guardrail.ActionRaise:
			return guardrail.RaiseWithInfo(message, info), nil
		default:
			return guardrail.RejectWithInfo(message, info), nil
		}
	}

	return guardrail.Allow(), nil
}

// Ensure PIIGuardrail implements the interface.
var _ guardrail.Guardrail[string] = (*PIIGuardrail)(nil)
