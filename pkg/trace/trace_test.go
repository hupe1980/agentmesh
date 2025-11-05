package trace_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/trace"
)

// TestNoopTracer verifies the no-op implementation doesn't panic
func TestNoopTracer(t *testing.T) {
	ctx := context.Background()
	provider := trace.Noop()

	tracer := provider.Tracer("test_tracer")
	ctx, span := tracer.Start(ctx, "operation")

	// Should not panic
	span.End(nil)
	span.End(errors.New("test error"))

	// Should be able to nest spans
	ctx2, span2 := tracer.Start(ctx, "nested_operation")
	span2.End(nil)

	_ = ctx2 // Use context to avoid unused variable
}

// TestContextPropagation verifies tracer provider is stored and retrieved
func TestContextPropagation(t *testing.T) {
	ctx := context.Background()

	// Without provider, should get no-op
	provider1 := trace.FromContext(ctx)
	if provider1 == nil {
		t.Fatal("FromContext should never return nil")
	}

	// With provider
	mockProvider := &mockTraceProvider{}
	ctx = trace.WithProvider(ctx, mockProvider)

	provider2 := trace.FromContext(ctx)
	if provider2 != mockProvider {
		t.Error("Should retrieve the same provider from context")
	}
}

// TestNilProviderDefaults verifies nil provider defaults to no-op
func TestNilProviderDefaults(t *testing.T) {
	ctx := context.Background()
	ctx = trace.WithProvider(ctx, nil)

	provider := trace.FromContext(ctx)
	if provider == nil {
		t.Fatal("WithProvider(nil) should default to no-op provider")
	}

	// Should not panic
	tracer := provider.Tracer("test")
	_, span := tracer.Start(ctx, "operation")
	span.End(nil)
}

// TestSpanLifecycle verifies span creation and ending
func TestSpanLifecycle(t *testing.T) {
	ctx := context.Background()
	mockProvider := &mockTraceProvider{}
	ctx = trace.WithProvider(ctx, mockProvider)

	tracer := mockProvider.Tracer("service")
	ctx2, span := tracer.Start(ctx, "operation")

	if ctx2 == ctx {
		t.Error("Start should return a new context")
	}

	testErr := errors.New("operation failed")
	span.End(testErr)

	mockProvider.mu.Lock()
	defer mockProvider.mu.Unlock()

	if mockProvider.startCalls != 1 {
		t.Errorf("Expected 1 start call, got %d", mockProvider.startCalls)
	}
	if mockProvider.endCalls != 1 {
		t.Errorf("Expected 1 end call, got %d", mockProvider.endCalls)
	}
	if !errors.Is(mockProvider.lastEndError, testErr) {
		t.Errorf("Expected end error %v, got %v", testErr, mockProvider.lastEndError)
	}
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
	defer mockProvider.mu.Unlock()

	if len(mockProvider.lastStartAttrs) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(mockProvider.lastStartAttrs))
	}
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
	defer mockProvider.mu.Unlock()
	if mockProvider.startCalls != 100 {
		t.Errorf("Expected 100 start calls, got %d", mockProvider.startCalls)
	}
	if mockProvider.endCalls != 100 {
		t.Errorf("Expected 100 end calls, got %d", mockProvider.endCalls)
	}
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

	if ctx1 == ctx2 {
		t.Error("Nested spans should have different contexts")
	}

	span2.End(nil)
	span1.End(nil)

	mockProvider.mu.Lock()
	defer mockProvider.mu.Unlock()
	if mockProvider.startCalls != 2 {
		t.Errorf("Expected 2 start calls for nested spans, got %d", mockProvider.startCalls)
	}
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
