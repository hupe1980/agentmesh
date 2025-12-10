package model

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// ComplexityEstimator estimates the complexity of a request on a scale of 0.0 to 1.0.
type ComplexityEstimator interface {
	// Estimate returns a complexity score between 0.0 (simple) and 1.0 (complex).
	Estimate(ctx context.Context, req *Request) (float64, error)
}

// CostBasedRouter routes requests to models based on estimated complexity.
// Simple requests go to cheaper/faster models, complex ones to more capable models.
type CostBasedRouter struct {
	cheapModel     Model
	expensiveModel Model
	threshold      float64 // Complexity threshold (0.0-1.0)
	estimator      ComplexityEstimator
}

// CostRouterOption configures a CostBasedRouter.
type CostRouterOption func(*CostBasedRouter)

// WithComplexityThreshold sets the threshold above which expensive model is used.
// Default is 0.3 (30% complexity).
func WithComplexityThreshold(threshold float64) CostRouterOption {
	return func(r *CostBasedRouter) {
		r.threshold = threshold
	}
}

// WithComplexityEstimator sets a custom complexity estimator.
func WithComplexityEstimator(e ComplexityEstimator) CostRouterOption {
	return func(r *CostBasedRouter) {
		r.estimator = e
	}
}

// NewCostBasedRouter creates a new cost-based router.
// The cheap model handles simple requests, expensive model handles complex ones.
func NewCostBasedRouter(cheap, expensive Model, opts ...CostRouterOption) *CostBasedRouter {
	r := &CostBasedRouter{
		cheapModel:     cheap,
		expensiveModel: expensive,
		threshold:      0.3, // Default: 30% complexity threshold
		estimator:      &HeuristicEstimator{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Route selects a model based on request complexity.
func (r *CostBasedRouter) Route(ctx context.Context, req *Request) (Model, error) {
	complexity, err := r.estimator.Estimate(ctx, req)
	if err != nil {
		// On estimation error, use expensive model (safe default).
		// We intentionally ignore the estimation error and fallback gracefully.
		complexity = 1.0 // Force expensive model selection
	}

	if complexity < r.threshold {
		return r.cheapModel, nil
	}
	return r.expensiveModel, nil
}

// HeuristicEstimator estimates complexity using simple heuristics.
// This provides a fast, no-API-call estimation based on message characteristics.
type HeuristicEstimator struct {
	// ComplexKeywords are words that indicate higher complexity.
	// Default set includes common analytical and technical terms.
	ComplexKeywords []string

	// WordCountWeight is the weight given to word count in complexity.
	// Higher values mean longer messages increase complexity more.
	WordCountWeight float64

	// ContextWeight is the weight given to conversation length.
	ContextWeight float64

	// ToolWeight is the additional complexity when tools are involved.
	ToolWeight float64

	// SchemaWeight is the additional complexity for structured output.
	SchemaWeight float64
}

// defaultComplexKeywords are words that typically indicate complex queries.
var defaultComplexKeywords = []string{
	// Analytical tasks
	"analyze", "compare", "contrast", "explain", "summarize", "evaluate",
	"synthesize", "critique", "assess", "investigate",
	// Technical tasks
	"code", "implement", "debug", "optimize", "refactor", "architecture",
	"algorithm", "data structure", "design pattern",
	// Mathematical/scientific
	"mathematical", "proof", "derive", "calculate", "equation", "theorem",
	"hypothesis", "statistical", "correlation",
	// Complex reasoning
	"reasoning", "logic", "inference", "deduce", "conclude", "implications",
	"trade-offs", "pros and cons", "advantages", "disadvantages",
}

// Estimate calculates complexity based on heuristics.
func (e *HeuristicEstimator) Estimate(ctx context.Context, req *Request) (float64, error) {
	if len(req.Messages) == 0 {
		return 0, nil
	}

	// Get keywords list
	keywords := e.ComplexKeywords
	if len(keywords) == 0 {
		keywords = defaultComplexKeywords
	}

	// Get weights with defaults
	wordWeight := e.WordCountWeight
	if wordWeight == 0 {
		wordWeight = 0.3 // Max 30% from word count
	}
	contextWeight := e.ContextWeight
	if contextWeight == 0 {
		contextWeight = 0.2 // Max 20% from context length
	}
	toolWeight := e.ToolWeight
	if toolWeight == 0 {
		toolWeight = 0.15 // 15% added for tools
	}
	schemaWeight := e.SchemaWeight
	if schemaWeight == 0 {
		schemaWeight = 0.1 // 10% added for structured output
	}

	complexity := 0.0

	// Analyze the last message (most relevant for complexity)
	lastMsg := req.Messages[len(req.Messages)-1]
	text := lastMsg.String()
	lowerText := strings.ToLower(text)

	// 1. Word count contribution (longer = more complex, up to 100 words)
	wordCount := len(strings.Fields(text))
	complexity += math.Min(float64(wordCount)/100, 1.0) * wordWeight

	// 2. Keyword contribution
	keywordHits := 0
	for _, kw := range keywords {
		if strings.Contains(lowerText, kw) {
			keywordHits++
		}
	}
	// Cap at 5 keyword hits contributing 30%
	complexity += math.Min(float64(keywordHits)*0.06, 0.3)

	// 3. Context length contribution (more turns = more complexity)
	contextLen := len(req.Messages)
	complexity += math.Min(float64(contextLen)*0.05, contextWeight)

	// 4. Tools contribution
	if len(req.Tools) > 0 {
		complexity += toolWeight
	}

	// 5. Structured output contribution
	if req.OutputSchema != nil {
		complexity += schemaWeight
	}

	// 6. Code block detection (code tasks are typically complex)
	if strings.Contains(text, "```") || strings.Contains(lowerText, "write code") {
		complexity += 0.15
	}

	// 7. Question complexity indicators
	questionIndicators := []string{"why", "how does", "explain", "what if", "could you"}
	for _, qi := range questionIndicators {
		if strings.Contains(lowerText, qi) {
			complexity += 0.05
			break
		}
	}

	return math.Min(complexity, 1.0), nil
}

// MLComplexityEstimator uses a lightweight model to estimate complexity.
// This provides more accurate estimation at the cost of an API call.
type MLComplexityEstimator struct {
	classifier Model
}

// NewMLComplexityEstimator creates a new ML-based complexity estimator.
// The classifier should be a fast, cheap model (e.g., GPT-4o-mini).
func NewMLComplexityEstimator(classifier Model) *MLComplexityEstimator {
	return &MLComplexityEstimator{classifier: classifier}
}

// Estimate uses the classifier model to estimate complexity.
func (e *MLComplexityEstimator) Estimate(ctx context.Context, req *Request) (float64, error) {
	if len(req.Messages) == 0 {
		return 0, nil
	}

	lastMsg := req.Messages[len(req.Messages)-1]
	prompt := `You are a query complexity classifier. Analyze the following query and respond with ONLY a single decimal number between 0.0 and 1.0 representing its complexity.

0.0 = Very simple (basic facts, greetings, simple math)
0.3 = Simple (short answers, lookups, simple questions)
0.5 = Moderate (explanations, comparisons, some reasoning)
0.7 = Complex (analysis, code, multi-step reasoning)
1.0 = Very complex (research, architecture, deep analysis)

Query: ` + lastMsg.String() + `

Complexity score:`

	classifyReq := &Request{
		Messages: []message.Message{message.NewHumanMessageFromText(prompt)},
	}

	resp, err := Last(e.classifier.Generate(ctx, classifyReq))
	if err != nil {
		return 0.5, err // Default to medium complexity on error
	}

	content := strings.TrimSpace(resp.Message.String())

	// Parse the response as a float
	var complexity float64
	_, parseErr := fmt.Sscanf(content, "%f", &complexity)
	if parseErr != nil {
		// Parse error is expected if model returns non-numeric response.
		// We intentionally ignore and use a safe default.
		complexity = 0.5
	}

	return math.Max(0, math.Min(1, complexity)), nil
}
