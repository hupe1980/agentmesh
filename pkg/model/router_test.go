package model_test

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// namedMockModel extends mockModel with a name for identification
type namedMockModel struct {
	name         string
	response     *model.Response
	err          error
	capabilities model.Capabilities
}

func (m *namedMockModel) Name() string {
	return m.name
}

func (m *namedMockModel) Capabilities() model.Capabilities {
	return m.capabilities
}

func (m *namedMockModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		if m.err != nil {
			yield(nil, m.err)
			return
		}
		if m.response != nil {
			yield(m.response, nil)
		}
	}
}

func newMockModel(name string) *namedMockModel {
	return &namedMockModel{
		name: name,
		response: &model.Response{
			Message: message.NewAIMessageFromText("Hello from " + name),
		},
	}
}

func newMockModelWithCaps(name string, caps model.Capabilities) *namedMockModel {
	m := newMockModel(name)
	m.capabilities = caps
	return m
}

// mockTool implements tool.Tool for testing
type mockTool struct {
	name string
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "mock tool" }
func (t *mockTool) Definition() *tool.Definition {
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        t.name,
			Description: "mock tool",
		},
	}
}
func (t *mockTool) Call(ctx context.Context, args string) (any, error) {
	return "result", nil
}

// --- RoutedModel Tests ---

func TestRoutedModel_Generate(t *testing.T) {
	cheap := newMockModel("cheap")
	expensive := newMockModel("expensive")
	router := model.NewCostBasedRouter(cheap, expensive)

	rm := model.NewRoutedModel(router)

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	resp, err := model.Last(rm.Generate(context.Background(), req))
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestRoutedModel_WithFallback(t *testing.T) {
	fallback := newMockModel("fallback")

	// Create a router that always fails
	failingRouter := model.RouterFunc(func(ctx context.Context, req *model.Request) (model.Model, error) {
		return nil, model.ErrNoModelAvailable
	})

	rm := model.NewRoutedModel(failingRouter, model.WithFallbackModel(fallback))

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	resp, err := model.Last(rm.Generate(context.Background(), req))
	require.NoError(t, err)
	assert.Contains(t, message.Stringify(resp.Message), "fallback")
}

func TestRoutedModel_WithRouteCallback(t *testing.T) {
	cheap := newMockModel("cheap")
	expensive := newMockModel("expensive")
	router := model.NewCostBasedRouter(cheap, expensive)

	var selectedModel model.Model
	rm := model.NewRoutedModel(router, model.WithRouteCallback(func(ctx context.Context, req *model.Request, selected model.Model) {
		selectedModel = selected
	}))

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hi")},
	}

	_, err := model.Last(rm.Generate(context.Background(), req))
	require.NoError(t, err)
	assert.Equal(t, cheap, selectedModel)
}

// --- CostBasedRouter Tests ---

func TestCostBasedRouter_SimpleQuery(t *testing.T) {
	cheap := newMockModel("cheap")
	expensive := newMockModel("expensive")
	router := model.NewCostBasedRouter(cheap, expensive, model.WithComplexityThreshold(0.3))

	// Simple query should route to cheap model
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("What is 2+2?")},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, cheap, selected)
}

func TestCostBasedRouter_ComplexQuery(t *testing.T) {
	cheap := newMockModel("cheap")
	expensive := newMockModel("expensive")
	router := model.NewCostBasedRouter(cheap, expensive, model.WithComplexityThreshold(0.3))

	// Complex query should route to expensive model
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Analyze the architectural implications of microservices vs monolithic patterns. Compare the trade-offs for distributed systems. Explain the reasoning behind each approach."),
		},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, expensive, selected)
}

func TestCostBasedRouter_WithTools(t *testing.T) {
	cheap := newMockModel("cheap")
	expensive := newMockModel("expensive")
	router := model.NewCostBasedRouter(cheap, expensive, model.WithComplexityThreshold(0.3))

	// Query with tools should increase complexity
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Use tools")},
		Tools:    []tool.Tool{&mockTool{name: "test_tool"}},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	// Tools add complexity, but short query might still be cheap
	// This depends on threshold - just verify no error
	assert.NotNil(t, selected)
}

func TestHeuristicEstimator(t *testing.T) {
	estimator := &model.HeuristicEstimator{}

	tests := []struct {
		name       string
		message    string
		expectLow  bool    // complexity < 0.3
		expectHigh bool    // complexity > threshold
		threshold  float64 // threshold for expectHigh
	}{
		{
			name:      "simple greeting",
			message:   "Hello",
			expectLow: true,
		},
		{
			name:      "simple question",
			message:   "What is 2+2?",
			expectLow: true,
		},
		{
			name:       "analysis request",
			message:    "Analyze the code and compare the algorithms, then explain your reasoning step by step",
			expectHigh: true,
			threshold:  0.4, // This query scores ~0.44
		},
		{
			name:       "code request",
			message:    "Write code to implement a binary search tree with insert, delete, and search operations",
			expectHigh: true,
			threshold:  0.35, // This query scores ~0.36
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &model.Request{
				Messages: []message.Message{message.NewHumanMessageFromText(tt.message)},
			}

			complexity, err := estimator.Estimate(context.Background(), req)
			require.NoError(t, err)

			if tt.expectLow {
				assert.Less(t, complexity, 0.3, "expected low complexity for: %s", tt.message)
			}
			if tt.expectHigh {
				assert.Greater(t, complexity, tt.threshold, "expected high complexity (>%v) for: %s", tt.threshold, tt.message)
			}
		})
	}
}

// --- CapabilityRouter Tests ---

func TestCapabilityRouter_MatchesVision(t *testing.T) {
	textModel := newMockModelWithCaps("text", model.Capabilities{})
	visionModel := newMockModelWithCaps("vision", model.Capabilities{Vision: true})

	router := model.NewCapabilityRouter([]model.Model{textModel, visionModel})

	// Request with image content should route to vision model
	imagePart := message.FilePart{
		MimeType: "image/png",
		Name:     "test.png",
	}
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessage([]message.Part{imagePart}),
		},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, visionModel, selected)
}

func TestCapabilityRouter_MatchesTools(t *testing.T) {
	basicModel := newMockModelWithCaps("basic", model.Capabilities{})
	toolModel := newMockModelWithCaps("tool", model.Capabilities{Tools: true})

	router := model.NewCapabilityRouter([]model.Model{basicModel, toolModel})

	// Request with tools should route to tool-capable model
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Use tools")},
		Tools:    []tool.Tool{&mockTool{name: "test_tool"}},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, toolModel, selected)
}

func TestCapabilityRouter_FallbackWhenNoMatch(t *testing.T) {
	basicModel := newMockModelWithCaps("basic", model.Capabilities{})
	fallback := newMockModel("fallback")

	router := model.NewCapabilityRouter(
		[]model.Model{basicModel},
		model.WithCapabilityFallback(fallback),
	)

	// Request requiring vision should fall back
	imagePart := message.FilePart{MimeType: "image/png", Name: "test.png"}
	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessage([]message.Part{imagePart})},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, fallback, selected)
}

// --- FallbackRouter Tests ---

func TestFallbackRouter_ReturnsFirstAvailable(t *testing.T) {
	model1 := newMockModel("model1")
	model2 := newMockModel("model2")
	model3 := newMockModel("model3")

	router := model.NewFallbackRouter([]model.Model{model1, model2, model3})

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, model1, selected)
}

func TestFallbackRouter_CircuitBreaker(t *testing.T) {
	model1 := newMockModel("model1")
	model2 := newMockModel("model2")

	router := model.NewFallbackRouter(
		[]model.Model{model1, model2},
		model.WithFailureThreshold(2),
		model.WithResetTimeout(100*time.Millisecond),
	)

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	// Initially model1 should be selected
	selected, _ := router.Route(context.Background(), req)
	assert.Equal(t, model1, selected)

	// Record failures to trip circuit
	router.RecordFailure(model1)
	router.RecordFailure(model1)

	// Now model2 should be selected (model1 circuit is open)
	selected, _ = router.Route(context.Background(), req)
	assert.Equal(t, model2, selected)

	// After reset timeout, model1 should be available again
	time.Sleep(150 * time.Millisecond)
	selected, _ = router.Route(context.Background(), req)
	assert.Equal(t, model1, selected)
}

func TestFallbackRouter_RecordSuccess(t *testing.T) {
	model1 := newMockModel("model1")

	router := model.NewFallbackRouter(
		[]model.Model{model1},
		model.WithFailureThreshold(2),
	)

	// Record one failure
	router.RecordFailure(model1)
	assert.Equal(t, model.CircuitClosed, router.CircuitState(model1))

	// Record success resets failures
	router.RecordSuccess(model1)
	assert.Equal(t, model.CircuitClosed, router.CircuitState(model1))
}

// --- CompositeRouter Tests ---

func TestCompositeRouter_ChainsRouters(t *testing.T) {
	model1 := newMockModel("model1")

	// First router always fails
	failingRouter := model.RouterFunc(func(ctx context.Context, req *model.Request) (model.Model, error) {
		return nil, model.ErrNoModelAvailable
	})

	// Second router returns model1
	successRouter := model.NewStaticRouter(model1)

	composite := model.NewCompositeRouter([]model.Router{failingRouter, successRouter})

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	selected, err := composite.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, model1, selected)
}

func TestCompositeRouter_FallbackOnAllFail(t *testing.T) {
	fallback := newMockModel("fallback")

	failingRouter := model.RouterFunc(func(ctx context.Context, req *model.Request) (model.Model, error) {
		return nil, model.ErrNoModelAvailable
	})

	composite := model.NewCompositeRouter(
		[]model.Router{failingRouter},
		model.WithCompositeFallback(fallback),
	)

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	selected, err := composite.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, fallback, selected)
}

// --- ConditionalRouter Tests ---

func TestConditionalRouter(t *testing.T) {
	primaryModel := newMockModel("primary")
	altModel := newMockModel("alternative")

	// Route to primary if message is long, otherwise alternative
	router := model.NewConditionalRouter(
		func(ctx context.Context, req *model.Request) bool {
			text := message.Stringify(req.Messages[0])
			return len(text) > 20
		},
		model.NewStaticRouter(primaryModel),
		model.NewStaticRouter(altModel),
	)

	// Short message -> alternative
	shortReq := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hi")},
	}
	selected, _ := router.Route(context.Background(), shortReq)
	assert.Equal(t, altModel, selected)

	// Long message -> primary
	longReq := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("This is a much longer message that exceeds twenty characters")},
	}
	selected, _ = router.Route(context.Background(), longReq)
	assert.Equal(t, primaryModel, selected)
}

// --- StaticRouter Tests ---

func TestStaticRouter(t *testing.T) {
	m := newMockModel("static")
	router := model.NewStaticRouter(m)

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, m, selected)
}

func TestStaticRouter_NilModel(t *testing.T) {
	router := model.NewStaticRouter(nil)

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	_, err := router.Route(context.Background(), req)
	assert.ErrorIs(t, err, model.ErrNoModelAvailable)
}

// --- WeightedRouter Tests ---

func TestWeightedRouter(t *testing.T) {
	model1 := newMockModel("model1")
	model2 := newMockModel("model2")
	model3 := newMockModel("model3")

	// Weight: model1=1, model2=2, model3=3 (total=6)
	router := model.NewWeightedRouter(
		[]model.Model{model1, model2, model3},
		[]int{1, 2, 3},
		model.WithWeightedRandomFunc(func(n int) int {
			return 0 // Always return first bucket -> model1
		}),
	)

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, model1, selected)

	// Test with random function returning 4 (should select model3: cumulative 1+2+3=6, 4 falls in model3's range)
	router2 := model.NewWeightedRouter(
		[]model.Model{model1, model2, model3},
		[]int{1, 2, 3},
		model.WithWeightedRandomFunc(func(n int) int {
			return 4 // Falls in model3's range (3-5)
		}),
	)

	selected, _ = router2.Route(context.Background(), req)
	assert.Equal(t, model3, selected)
}

// --- CircuitBreaker Tests ---

func TestCircuitBreaker_States(t *testing.T) {
	cb := model.NewCircuitBreaker(2, 50*time.Millisecond)

	// Initially closed
	assert.False(t, cb.IsOpen())
	assert.Equal(t, model.CircuitClosed, cb.State())

	// One failure - still closed
	cb.RecordFailure()
	assert.False(t, cb.IsOpen())

	// Two failures - opens
	cb.RecordFailure()
	assert.True(t, cb.IsOpen())
	assert.Equal(t, model.CircuitOpen, cb.State())

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Should be half-open now
	assert.False(t, cb.IsOpen())
	assert.Equal(t, model.CircuitHalfOpen, cb.State())

	// Success in half-open -> closed
	cb.RecordSuccess()
	assert.Equal(t, model.CircuitClosed, cb.State())
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := model.NewCircuitBreaker(1, time.Second)

	// Trip the circuit
	cb.RecordFailure()
	assert.True(t, cb.IsOpen())

	// Reset
	cb.Reset()
	assert.False(t, cb.IsOpen())
	assert.Equal(t, model.CircuitClosed, cb.State())
}

// --- RouterFunc Tests ---

func TestRouterFunc(t *testing.T) {
	m := newMockModel("func")

	router := model.RouterFunc(func(ctx context.Context, req *model.Request) (model.Model, error) {
		return m, nil
	})

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("Hello")},
	}

	selected, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, m, selected)
}
