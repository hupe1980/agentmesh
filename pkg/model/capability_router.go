package model

import (
	"context"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// CapabilityRouter routes requests to models based on required capabilities.
// It inspects the request to determine what capabilities are needed and
// selects a model that supports all of them.
type CapabilityRouter struct {
	models   []Model
	fallback Model
	detector CapabilityDetector
}

// CapabilityRouterOption configures a CapabilityRouter.
type CapabilityRouterOption func(*CapabilityRouter)

// WithCapabilityDetector sets a custom capability detector.
func WithCapabilityDetector(d CapabilityDetector) CapabilityRouterOption {
	return func(r *CapabilityRouter) {
		r.detector = d
	}
}

// WithCapabilityFallback sets a fallback model when no model matches.
func WithCapabilityFallback(m Model) CapabilityRouterOption {
	return func(r *CapabilityRouter) {
		r.fallback = m
	}
}

// NewCapabilityRouter creates a new capability-based router.
// Models are checked in order; the first one satisfying all requirements is selected.
func NewCapabilityRouter(models []Model, opts ...CapabilityRouterOption) *CapabilityRouter {
	r := &CapabilityRouter{
		models:   models,
		detector: &DefaultCapabilityDetector{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Route selects a model based on required capabilities.
func (r *CapabilityRouter) Route(ctx context.Context, req *Request) (Model, error) {
	required, err := r.detector.Detect(ctx, req)
	if err != nil {
		if r.fallback != nil {
			return r.fallback, nil
		}
		return nil, err
	}

	// Find first model that supports all required capabilities
	for _, m := range r.models {
		caps := m.Capabilities()
		if satisfies(caps, required) {
			return m, nil
		}
	}

	if r.fallback != nil {
		return r.fallback, nil
	}

	return nil, ErrNoModelAvailable
}

// satisfies checks if 'has' capabilities satisfy 'needs' requirements.
func satisfies(has, needs Capabilities) bool {
	// Check boolean capability requirements
	boolChecks := []struct {
		needed, provided bool
	}{
		{needs.Vision, has.Vision},
		{needs.Tools, has.Tools},
		{needs.NativeReasoning, has.NativeReasoning},
		{needs.StructuredOutput, has.StructuredOutput},
		{needs.Streaming, has.Streaming},
		{needs.Audio, has.Audio},
		{needs.Logprobs, has.Logprobs},
	}

	for _, check := range boolChecks {
		if check.needed && !check.provided {
			return false
		}
	}

	// Check context window requirements
	if needs.MaxContextTokens > 0 && has.MaxContextTokens > 0 {
		if needs.MaxContextTokens > has.MaxContextTokens {
			return false
		}
	}

	return true
}

// CapabilityDetector detects required capabilities from a request.
type CapabilityDetector interface {
	// Detect returns the capabilities required to handle the request.
	Detect(ctx context.Context, req *Request) (Capabilities, error)
}

// DefaultCapabilityDetector detects capabilities from request content.
type DefaultCapabilityDetector struct {
	// ReasoningKeywords are words that indicate reasoning capability is needed.
	ReasoningKeywords []string

	// TokensPerWord is used to estimate token count (default: 1.3)
	TokensPerWord float64
}

// defaultReasoningKeywords indicate complex reasoning is needed.
var defaultReasoningKeywords = []string{
	"step by step", "think through", "reason about", "chain of thought",
	"explain your reasoning", "show your work", "logical", "deduce",
	"prove", "derive", "theorem", "proof",
}

// Detect analyzes the request to determine required capabilities.
func (d *DefaultCapabilityDetector) Detect(ctx context.Context, req *Request) (Capabilities, error) {
	var caps Capabilities

	tokensPerWord := d.TokensPerWord
	if tokensPerWord == 0 {
		tokensPerWord = 1.3
	}

	keywords := d.ReasoningKeywords
	if len(keywords) == 0 {
		keywords = defaultReasoningKeywords
	}

	totalWords := 0

	for _, msg := range req.Messages {
		// Check parts for file content (images, audio, etc.)
		for _, part := range msg.Parts() {
			if fp, ok := part.(message.FilePart); ok {
				// Check MIME type to determine capability needed
				if strings.HasPrefix(fp.MimeType, "image/") {
					caps.Vision = true
				}
				if strings.HasPrefix(fp.MimeType, "audio/") {
					caps.Audio = true
				}
			}

			// Check for function/tool calls in message parts
			if _, ok := part.(message.FunctionCallPart); ok {
				caps.Tools = true
			}
		}

		// Count words for token estimation
		text := msg.String()
		totalWords += len(strings.Fields(text))
		lowerText := strings.ToLower(text)

		// Check for reasoning indicators
		for _, kw := range keywords {
			if strings.Contains(lowerText, kw) {
				caps.NativeReasoning = true
				break
			}
		}
	}

	// Check if tools are provided in the request
	if len(req.Tools) > 0 {
		caps.Tools = true
	}

	// Check if structured output is requested
	if req.OutputSchema != nil {
		caps.StructuredOutput = true
	}

	// Check if streaming is requested
	if req.Stream {
		caps.Streaming = true
	}

	// Estimate token count for context window requirement
	estimatedTokens := int(float64(totalWords) * tokensPerWord)
	if estimatedTokens > 8000 {
		caps.MaxContextTokens = estimatedTokens
	}

	return caps, nil
}

// CapabilityScore calculates a score for how well a model matches requirements.
// Higher score means better match. Returns 0 if requirements are not met.
func CapabilityScore(has, needs Capabilities) int {
	if !satisfies(has, needs) {
		return 0
	}

	score := 100 // Base score for meeting requirements

	// Bonus for extra capabilities
	if has.Vision && !needs.Vision {
		score += 5
	}
	if has.Tools && !needs.Tools {
		score += 5
	}
	if has.NativeReasoning && !needs.NativeReasoning {
		score += 10
	}
	if has.StructuredOutput && !needs.StructuredOutput {
		score += 5
	}

	// Bonus for larger context window
	if has.MaxContextTokens > needs.MaxContextTokens {
		score += 5
	}

	return score
}

// BestMatchRouter routes to the model with the highest capability score.
type BestMatchRouter struct {
	models   []Model
	fallback Model
	detector CapabilityDetector
}

// NewBestMatchRouter creates a router that selects the best matching model.
func NewBestMatchRouter(models []Model, opts ...CapabilityRouterOption) *BestMatchRouter {
	r := &BestMatchRouter{
		models:   models,
		detector: &DefaultCapabilityDetector{},
	}
	// Apply same options as CapabilityRouter
	for _, opt := range opts {
		// Type assertion hack to reuse options
		cr := &CapabilityRouter{}
		opt(cr)
		r.detector = cr.detector
		if cr.fallback != nil {
			r.fallback = cr.fallback
		}
	}
	return r
}

// Route selects the model with the highest capability score.
func (r *BestMatchRouter) Route(ctx context.Context, req *Request) (Model, error) {
	required, err := r.detector.Detect(ctx, req)
	if err != nil {
		if r.fallback != nil {
			return r.fallback, nil
		}
		return nil, err
	}

	var bestModel Model
	bestScore := 0

	for _, m := range r.models {
		caps := m.Capabilities()
		score := CapabilityScore(caps, required)
		if score > bestScore {
			bestScore = score
			bestModel = m
		}
	}

	if bestModel != nil {
		return bestModel, nil
	}

	if r.fallback != nil {
		return r.fallback, nil
	}

	return nil, ErrNoModelAvailable
}
