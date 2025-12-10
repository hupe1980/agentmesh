package testutil

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// GenerateFunc is the signature for custom model generation logic.
type GenerateFunc func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error]

// MockModel is a configurable mock implementation of model.Model.
// Can be used directly with struct literal syntax for backward compatibility:
//
//	mdl := &testutil.MockModel{
//	    GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] { ... },
//	}
//
// Or use the builder pattern for more complex scenarios:
//
//	mdl := testutil.NewModelBuilder().WithResponse("Hello").Build()
type MockModel struct {
	// GenerateFunc is the function called by Generate. Can be set directly for simple mocks.
	GenerateFunc GenerateFunc
	// CapabilitiesFunc is the function called by Capabilities. Can be set directly for simple mocks.
	CapabilitiesFunc func() model.Capabilities
}

// Generate returns a sequence of model responses.
func (m *MockModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(ctx, req)
	}
	// Default implementation returns a single message
	return func(yield func(*model.Response, error) bool) {
		yield(&model.Response{
			Message: message.NewAIMessageFromText("mock response"),
			Partial: false,
		}, nil)
	}
}

// Capabilities returns the model's capabilities.
func (m *MockModel) Capabilities() model.Capabilities {
	if m.CapabilitiesFunc != nil {
		return m.CapabilitiesFunc()
	}
	return model.Capabilities{
		Streaming:           true,
		Tools:               true,
		MaxContextTokens:    4096,
		MaxOutputTokens:     2048,
		SupportedModalities: []string{"text"},
	}
}

// ModelBuilder provides a fluent API for building MockModel instances.
type ModelBuilder struct {
	mu           sync.Mutex
	responses    []modelStep
	capabilities model.Capabilities
	delay        time.Duration
	streaming    bool
	recorder     *ConversationRecorder
	customGen    GenerateFunc
}

type modelStep struct {
	text      string
	toolCalls []message.ToolCall
	err       error
}

// NewModelBuilder creates a new ModelBuilder with default settings.
func NewModelBuilder() *ModelBuilder {
	return &ModelBuilder{
		capabilities: model.Capabilities{
			Streaming:           true,
			Tools:               true,
			MaxContextTokens:    4096,
			MaxOutputTokens:     2048,
			SupportedModalities: []string{"text"},
		},
	}
}

// WithResponse adds a text response to the model.
func (b *ModelBuilder) WithResponse(text string) *ModelBuilder {
	b.responses = append(b.responses, modelStep{text: text})
	return b
}

// WithResponses adds multiple sequential text responses.
func (b *ModelBuilder) WithResponses(texts ...string) *ModelBuilder {
	for _, text := range texts {
		b.responses = append(b.responses, modelStep{text: text})
	}
	return b
}

// WithToolCalls adds a response with tool calls.
func (b *ModelBuilder) WithToolCalls(calls ...message.ToolCall) *ModelBuilder {
	b.responses = append(b.responses, modelStep{toolCalls: calls})
	return b
}

// WithError configures the model to return an error.
func (b *ModelBuilder) WithError(err error) *ModelBuilder {
	b.responses = append(b.responses, modelStep{err: err})
	return b
}

// WithDelay adds a delay before each response (for timeout testing).
func (b *ModelBuilder) WithDelay(d time.Duration) *ModelBuilder {
	b.delay = d
	return b
}

// WithStreaming enables streaming mode (responses are chunked).
func (b *ModelBuilder) WithStreaming(enabled bool) *ModelBuilder {
	b.streaming = enabled
	return b
}

// WithCapabilities sets the model capabilities.
func (b *ModelBuilder) WithCapabilities(caps model.Capabilities) *ModelBuilder {
	b.capabilities = caps
	return b
}

// WithTools enables or disables tool support.
func (b *ModelBuilder) WithTools(enabled bool) *ModelBuilder {
	b.capabilities.Tools = enabled
	return b
}

// WithStructuredOutput enables or disables structured output support.
func (b *ModelBuilder) WithStructuredOutput(enabled bool) *ModelBuilder {
	b.capabilities.StructuredOutput = enabled
	return b
}

// WithRecorder attaches a conversation recorder.
func (b *ModelBuilder) WithRecorder(r *ConversationRecorder) *ModelBuilder {
	b.recorder = r
	return b
}

// WithGenerator sets a custom generate function (for advanced use cases).
func (b *ModelBuilder) WithGenerator(fn GenerateFunc) *ModelBuilder {
	b.customGen = fn
	return b
}

// Build creates the MockModel with the configured settings.
func (b *ModelBuilder) Build() *MockModel {
	// If custom generator is set, use it directly
	if b.customGen != nil {
		return b.buildWithCustomGenerator()
	}

	// If no responses configured, add a default
	if len(b.responses) == 0 {
		b.responses = append(b.responses, modelStep{text: "mock response"})
	}

	return b.buildWithResponses()
}

// buildWithCustomGenerator creates a MockModel using a custom generator function.
func (b *ModelBuilder) buildWithCustomGenerator() *MockModel {
	return &MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			if b.recorder != nil {
				b.recorder.RecordRequest(req)
			}
			return b.customGen(ctx, req)
		},
		CapabilitiesFunc: func() model.Capabilities { return b.capabilities },
	}
}

// buildWithResponses creates a MockModel using the configured responses.
func (b *ModelBuilder) buildWithResponses() *MockModel {
	idx := 0

	return &MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				if b.recorder != nil {
					b.recorder.RecordRequest(req)
				}

				if !b.handleDelay(ctx, yield) {
					return
				}

				step := b.getNextStep(&idx)

				if step.err != nil {
					yield(nil, step.err)
					return
				}

				b.yieldResponse(step, yield)
			}
		},
		CapabilitiesFunc: func() model.Capabilities { return b.capabilities },
	}
}

// handleDelay waits for the configured delay, returning false if context is cancelled.
func (b *ModelBuilder) handleDelay(ctx context.Context, yield func(*model.Response, error) bool) bool {
	if b.delay > 0 {
		select {
		case <-time.After(b.delay):
		case <-ctx.Done():
			yield(nil, ctx.Err())
			return false
		}
	}
	return true
}

// getNextStep returns the next response step, repeating the last one if exhausted.
func (b *ModelBuilder) getNextStep(idx *int) modelStep {
	b.mu.Lock()
	defer b.mu.Unlock()

	currentIdx := *idx
	if currentIdx >= len(b.responses) {
		currentIdx = len(b.responses) - 1
	}
	*idx++
	return b.responses[currentIdx]
}

// yieldResponse yields the response, handling streaming if enabled.
func (b *ModelBuilder) yieldResponse(step modelStep, yield func(*model.Response, error) bool) {
	msg := message.NewAIMessageFromText(step.text)
	if len(step.toolCalls) > 0 {
		msg.ToolCalls = step.toolCalls
	}

	// Handle streaming
	if b.streaming && len(step.text) > 3 {
		if !b.yieldStreamingChunks(step.text, yield) {
			return
		}
	}

	// Yield final response
	resp := &model.Response{Message: msg, Partial: false}
	if b.recorder != nil {
		b.recorder.RecordResponse(resp)
	}
	yield(resp, nil)
}

// yieldStreamingChunks yields text in chunks for streaming mode.
func (b *ModelBuilder) yieldStreamingChunks(text string, yield func(*model.Response, error) bool) bool {
	chunkSize := len(text) / 3
	for i := 0; i < len(text); i += chunkSize {
		end := min(i+chunkSize, len(text))
		chunk := message.NewAIMessageFromText(text[i:end])
		resp := &model.Response{Message: chunk, Partial: true}
		if b.recorder != nil {
			b.recorder.RecordResponse(resp)
		}
		if !yield(resp, nil) {
			return false
		}
	}
	return true
}

// WrapSimpleGenerate wraps a simple generate function into an iterator for custom generators.
func WrapSimpleGenerate(fn func(ctx context.Context, messages []message.Message) (message.Message, error)) GenerateFunc {
	return func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			msg, err := fn(ctx, req.Messages)
			yield(&model.Response{Message: msg, Partial: false}, err)
		}
	}
}
