package trace_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoopTracer verifies the no-op implementation doesn't panic
func TestNoopTracer(t *testing.T) {
	ctx := context.Background()
	provider := trace.Noop()

	tracer := provider.Tracer("test_tracer")
	ctx, span := tracer.Start(ctx, "operation")

	// Should not panic
	assert.NotPanics(t, func() {
		span.End(nil)
		span.End(errors.New("test error"))
	})

	// Should be able to nest spans
	assert.NotPanics(t, func() {
		ctx2, span2 := tracer.Start(ctx, "nested_operation")
		span2.End(nil)
		_ = ctx2 // Use context to avoid unused variable
	})
}

// TestContextPropagation verifies tracer provider is stored and retrieved
func TestContextPropagation(t *testing.T) {
	ctx := context.Background()

	// Without provider, should get no-op
	provider1 := trace.FromContext(ctx)
	require.NotNil(t, provider1, "FromContext should never return nil")

	// With provider
	mockProvider := &mockTraceProvider{}
	ctx = trace.WithProvider(ctx, mockProvider)

	provider2 := trace.FromContext(ctx)
	assert.Equal(t, mockProvider, provider2, "Should retrieve the same provider from context")
}

// TestNilProviderDefaults verifies nil provider defaults to no-op
func TestNilProviderDefaults(t *testing.T) {
	ctx := context.Background()
	ctx = trace.WithProvider(ctx, nil)

	provider := trace.FromContext(ctx)
	require.NotNil(t, provider, "WithProvider(nil) should default to no-op provider")

	// Should not panic
	assert.NotPanics(t, func() {
		tracer := provider.Tracer("test")
		_, span := tracer.Start(ctx, "operation")
		span.End(nil)
	})
}

// TestSpanLifecycle verifies span creation and ending
func TestSpanLifecycle(t *testing.T) {
	ctx := context.Background()
	mockProvider := &mockTraceProvider{}
	ctx = trace.WithProvider(ctx, mockProvider)

	tracer := mockProvider.Tracer("service")
	ctx2, span := tracer.Start(ctx, "operation")

	assert.NotEqual(t, ctx, ctx2, "Start should return a new context")

	testErr := errors.New("operation failed")
	span.End(testErr)

	mockProvider.mu.Lock()
	startCalls := mockProvider.startCalls
	endCalls := mockProvider.endCalls
	endErr := mockProvider.lastEndError
	mockProvider.mu.Unlock()

	assert.Equal(t, 1, startCalls)
	assert.Equal(t, 1, endCalls)
	assert.ErrorIs(t, endErr, testErr)
}

// TestSpanAttributes verifies attributes are passed correctly
func TestSpanAttributes(t *testing.T) {
	ctx := context.Background()
	mockProvider := &mockTraceProvider{}
	tracer := mockProvider.Tracer("service")

	_, span := tracer.Start(ctx, "operation",
		trace.Attr{Key: "user_id", Value: "123"},
		trace.Attr{Key: "request_id", Value: "abc"},
	)
	span.End(nil)

	mockProvider.mu.Lock()
	attrs := mockProvider.lastStartAttrs
	mockProvider.mu.Unlock()

	require.Len(t, attrs, 2)
	assert.Equal(t, "user_id", attrs[0].Key)
	assert.Equal(t, "123", attrs[0].Value)
	assert.Equal(t, "request_id", attrs[1].Key)
	assert.Equal(t, "abc", attrs[1].Value)
}

// TestConcurrentTracing verifies thread safety
func TestConcurrentTracing(t *testing.T) {
	ctx := context.Background()
	mockProvider := &mockTraceProvider{}
	ctx = trace.WithProvider(ctx, mockProvider)

	tracer := mockProvider.Tracer("concurrent_service")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, span := tracer.Start(ctx, "concurrent_op")
			span.End(nil)
		}(i)
	}

	wg.Wait()

	mockProvider.mu.Lock()
	startCalls := mockProvider.startCalls
	endCalls := mockProvider.endCalls
	mockProvider.mu.Unlock()

	assert.Equal(t, 100, startCalls)
	assert.Equal(t, 100, endCalls)
}

// TestNestedSpans verifies nested span context propagation
func TestNestedSpans(t *testing.T) {
	ctx := context.Background()
	mockProvider := &mockTraceProvider{}
	tracer := mockProvider.Tracer("service")

	// Outer span
	ctx1, span1 := tracer.Start(ctx, "outer")

	// Inner span
	ctx2, span2 := tracer.Start(ctx1, "inner")

	assert.NotEqual(t, ctx1, ctx2, "Nested spans should have different contexts")

	span2.End(nil)
	span1.End(nil)

	mockProvider.mu.Lock()
	startCalls := mockProvider.startCalls
	mockProvider.mu.Unlock()

	assert.Equal(t, 2, startCalls, "Expected 2 start calls for nested spans")
}

// mockTraceProvider is a test implementation that tracks calls
type mockTraceProvider struct {
	mu             sync.Mutex
	startCalls     int
	endCalls       int
	lastSpanName   string
	lastStartAttrs []trace.Attr
	lastEndError   error
}

func (m *mockTraceProvider) Tracer(name string) trace.Tracer {
	return &mockTracer{provider: m}
}

type mockTracer struct {
	provider *mockTraceProvider
}

func (t *mockTracer) Start(ctx context.Context, name string, attrs ...trace.Attr) (context.Context, trace.Span) {
	t.provider.mu.Lock()
	defer t.provider.mu.Unlock()

	t.provider.startCalls++
	t.provider.lastSpanName = name
	t.provider.lastStartAttrs = attrs

	span := &mockSpan{provider: t.provider}
	return context.WithValue(ctx, "span", span), span
}

type mockSpan struct {
	provider *mockTraceProvider
}

func (s *mockSpan) End(err error) {
	s.provider.mu.Lock()
	defer s.provider.mu.Unlock()

	s.provider.endCalls++
	s.provider.lastEndError = err
}
