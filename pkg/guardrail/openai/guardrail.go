// Package openai provides a guardrail implementation using OpenAI's Moderation API.
package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/openai/openai-go/v2"
)

// Category represents an OpenAI moderation category.
type Category string

// OpenAI moderation categories.
const (
	CategoryHate                  Category = "hate"
	CategoryHateThreatening       Category = "hate/threatening"
	CategoryHarassment            Category = "harassment"
	CategoryHarassmentThreatening Category = "harassment/threatening"
	CategorySelfHarm              Category = "self-harm"
	CategorySelfHarmIntent        Category = "self-harm/intent"
	CategorySelfHarmInstructions  Category = "self-harm/instructions"
	CategorySexual                Category = "sexual"
	CategorySexualMinors          Category = "sexual/minors"
	CategoryViolence              Category = "violence"
	CategoryViolenceGraphic       Category = "violence/graphic"
	CategoryIllicit               Category = "illicit"
	CategoryIllicitViolent        Category = "illicit/violent"
)

// Client defines the interface for interacting with the OpenAI Moderation API.
// This interface allows for easy mocking in tests.
type Client interface {
	Moderate(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error)
}

// ClientWrapper wraps an OpenAI client to implement the Client interface.
type ClientWrapper struct {
	inner *openai.Client
}

// NewClientWrapper creates a new ClientWrapper from an OpenAI client.
func NewClientWrapper(client *openai.Client) *ClientWrapper {
	return &ClientWrapper{inner: client}
}

// Moderate calls the OpenAI Moderation API.
func (c *ClientWrapper) Moderate(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
	return c.inner.Moderations.New(ctx, params)
}

// Guardrail implements the guardrail.Guardrail interface using OpenAI's Moderation API.
type Guardrail struct {
	client     Client
	name       string
	action     guardrail.Action
	model      openai.ModerationModel
	thresholds map[Category]float64
}

// Options configures the OpenAI guardrail.
type Options struct {
	// Name is the guardrail name for identification.
	Name string

	// Action specifies what to do when content is flagged.
	Action guardrail.Action

	// Model specifies the moderation model to use.
	Model openai.ModerationModel

	// Thresholds specifies custom thresholds per category.
	// If a category score exceeds its threshold, it's flagged.
	Thresholds map[Category]float64
}

// Option is a function that configures the guardrail.
type Option func(*Options)

// WithName sets a custom name for the guardrail.
func WithName(name string) Option {
	return func(o *Options) {
		o.Name = name
	}
}

// WithAction sets the action to take when content is flagged.
func WithAction(action guardrail.Action) Option {
	return func(o *Options) {
		o.Action = action
	}
}

// WithModel sets the moderation model to use.
func WithModel(model openai.ModerationModel) Option {
	return func(o *Options) {
		o.Model = model
	}
}

// WithThreshold sets a custom threshold for a specific category.
func WithThreshold(category Category, threshold float64) Option {
	return func(o *Options) {
		if o.Thresholds == nil {
			o.Thresholds = make(map[Category]float64)
		}
		o.Thresholds[category] = threshold
	}
}

// WithThresholds sets custom thresholds for multiple categories.
func WithThresholds(thresholds map[Category]float64) Option {
	return func(o *Options) {
		o.Thresholds = thresholds
	}
}

// New creates a new OpenAI moderation guardrail from a Client.
func New(client Client, opts ...Option) *Guardrail {
	options := &Options{
		Name:       "openai-moderation",
		Action:     guardrail.ActionReject,
		Model:      openai.ModerationModelOmniModerationLatest,
		Thresholds: make(map[Category]float64),
	}

	for _, opt := range opts {
		opt(options)
	}

	return &Guardrail{
		client:     client,
		name:       options.Name,
		action:     options.Action,
		model:      options.Model,
		thresholds: options.Thresholds,
	}
}

// Name returns the guardrail name.
func (g *Guardrail) Name() string {
	return g.name
}

// Check checks the input text using OpenAI's Moderation API.
func (g *Guardrail) Check(ctx context.Context, input string) (*guardrail.Result, error) {
	params := openai.ModerationNewParams{
		Input: openai.ModerationNewParamsInputUnion{
			OfString: openai.String(input),
		},
		Model: g.model,
	}

	resp, err := g.client.Moderate(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai moderation api error: %w", err)
	}

	if len(resp.Results) == 0 {
		return guardrail.Allow(), nil
	}

	result := resp.Results[0]
	flaggedCategories := g.getFlaggedCategories(result)

	if len(flaggedCategories) > 0 {
		message := fmt.Sprintf("content flagged for: %s", strings.Join(flaggedCategories, ", "))
		info := map[string]any{
			"categories":      flaggedCategories,
			"category_scores": g.getCategoryScoresMap(result.CategoryScores),
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

// getFlaggedCategories returns a list of categories that were flagged.
func (g *Guardrail) getFlaggedCategories(result openai.Moderation) []string {
	var flagged []string

	scores := g.getCategoryScoresMap(result.CategoryScores)
	categories := g.getCategoriesMap(result.Categories)

	for category, score := range scores {
		threshold, hasThreshold := g.thresholds[Category(category)]
		if hasThreshold {
			// Use custom threshold
			if score >= threshold {
				flagged = append(flagged, category)
			}
		} else if categories[category] {
			// Use OpenAI's default flagging
			flagged = append(flagged, category)
		}
	}

	return flagged
}

// getCategoryScoresMap converts ModerationCategoryScores to a map.
//
//nolint:dupl // Similar structure to getCategoriesMap but different types (float64 vs bool)
func (g *Guardrail) getCategoryScoresMap(scores openai.ModerationCategoryScores) map[string]float64 {
	return map[string]float64{
		string(CategoryHate):                  scores.Hate,
		string(CategoryHateThreatening):       scores.HateThreatening,
		string(CategoryHarassment):            scores.Harassment,
		string(CategoryHarassmentThreatening): scores.HarassmentThreatening,
		string(CategorySelfHarm):              scores.SelfHarm,
		string(CategorySelfHarmIntent):        scores.SelfHarmIntent,
		string(CategorySelfHarmInstructions):  scores.SelfHarmInstructions,
		string(CategorySexual):                scores.Sexual,
		string(CategorySexualMinors):          scores.SexualMinors,
		string(CategoryViolence):              scores.Violence,
		string(CategoryViolenceGraphic):       scores.ViolenceGraphic,
		string(CategoryIllicit):               scores.Illicit,
		string(CategoryIllicitViolent):        scores.IllicitViolent,
	}
}

// getCategoriesMap converts ModerationCategories to a map.
//
//nolint:dupl // Similar structure to getCategoryScoresMap but different types (bool vs float64)
func (g *Guardrail) getCategoriesMap(cats openai.ModerationCategories) map[string]bool {
	return map[string]bool{
		string(CategoryHate):                  cats.Hate,
		string(CategoryHateThreatening):       cats.HateThreatening,
		string(CategoryHarassment):            cats.Harassment,
		string(CategoryHarassmentThreatening): cats.HarassmentThreatening,
		string(CategorySelfHarm):              cats.SelfHarm,
		string(CategorySelfHarmIntent):        cats.SelfHarmIntent,
		string(CategorySelfHarmInstructions):  cats.SelfHarmInstructions,
		string(CategorySexual):                cats.Sexual,
		string(CategorySexualMinors):          cats.SexualMinors,
		string(CategoryViolence):              cats.Violence,
		string(CategoryViolenceGraphic):       cats.ViolenceGraphic,
		string(CategoryIllicit):               cats.Illicit,
		string(CategoryIllicitViolent):        cats.IllicitViolent,
	}
}

// Ensure Guardrail implements the interface.
var _ guardrail.Guardrail[string] = (*Guardrail)(nil)
