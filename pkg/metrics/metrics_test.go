package metrics_test

import (
	"context"
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/metrics"
)

// TestNoopProvider verifies the no-op implementation doesn't panic
func TestNoopProvider(t *testing.T) {
	ctx := context.Background()
	provider := metrics.Noop()

	counter := provider.Counter("test_counter")
	histogram := provider.Histogram("test_histogram")

	// Should not panic
	counter.Add(ctx, 1.0)
	counter.Add(ctx, 5.0, metrics.Attr{Key: "label", Value: "test"})
	histogram.Record(ctx, 100.0)
	histogram.Record(ctx, 200.0, metrics.Attr{Key: "bucket", Value: "slow"})
}

// TestContextPropagation verifies provider is stored and retrieved from context
func TestContextPropagation(t *testing.T) {
	ctx := context.Background()

	// Without provider, should get no-op
	provider1 := metrics.FromContext(ctx)
	if provider1 == nil {
		t.Fatal("FromContext should never return nil")
	}

	// With provider
	mockProvider := &mockMetricsProvider{}
	ctx = metrics.WithProvider(ctx, mockProvider)

	provider2 := metrics.FromContext(ctx)
	if provider2 != mockProvider {
		t.Error("Should retrieve the same provider from context")
	}
}

// TestNilProviderDefaults verifies nil provider defaults to no-op
func TestNilProviderDefaults(t *testing.T) {
	ctx := context.Background()
	ctx = metrics.WithProvider(ctx, nil)

	provider := metrics.FromContext(ctx)
	if provider == nil {
		t.Fatal("WithProvider(nil) should default to no-op provider")
	}

	// Should not panic
	counter := provider.Counter("test")
	counter.Add(ctx, 1.0)
}

// TestConcurrentAccess verifies thread safety of context operations
func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	provider := &mockMetricsProvider{}
	ctx = metrics.WithProvider(ctx, provider)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := metrics.FromContext(ctx)
			counter := p.Counter("concurrent_test")
			counter.Add(ctx, 1.0)
		}()
	}

	wg.Wait()

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.counterCalls != 100 {
		t.Errorf("Expected 100 counter calls, got %d", provider.counterCalls)
	}
}

// TestAttrCreation verifies Attr struct construction
func TestAttrCreation(t *testing.T) {
	attr := metrics.Attr{Key: "environment", Value: "production"}

	if attr.Key != "environment" {
		t.Errorf("Expected key 'environment', got %q", attr.Key)
	}
	if attr.Value != "production" {
		t.Errorf("Expected value 'production', got %v", attr.Value)
	}
}

// TestMultipleAttributes verifies multiple attributes can be passed
func TestMultipleAttributes(t *testing.T) {
	ctx := context.Background()
	provider := &mockMetricsProvider{}

	counter := provider.Counter("test")
	counter.Add(ctx, 1.0,
		metrics.Attr{Key: "region", Value: "us-east-1"},
		metrics.Attr{Key: "service", Value: "api"},
	)

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.lastCounterAttrs == nil || len(provider.lastCounterAttrs) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(provider.lastCounterAttrs))
	}
}

// mockMetricsProvider is a test implementation that tracks calls
type mockMetricsProvider struct {
	mu                 sync.Mutex
	counterCalls       int
	histogramCalls     int
	lastCounterValue   float64
	lastCounterAttrs   []metrics.Attr
	lastHistogramValue float64
}

func (m *mockMetricsProvider) Counter(name string) metrics.Counter {
	return &mockCounter{provider: m}
}

func (m *mockMetricsProvider) Histogram(name string) metrics.Histogram {
	return &mockHistogram{provider: m}
}

type mockCounter struct {
	provider *mockMetricsProvider
}

func (c *mockCounter) Add(ctx context.Context, value float64, attrs ...metrics.Attr) {
	c.provider.mu.Lock()
	defer c.provider.mu.Unlock()
	c.provider.counterCalls++
	c.provider.lastCounterValue = value
	c.provider.lastCounterAttrs = attrs
}

type mockHistogram struct {
	provider *mockMetricsProvider
}

func (h *mockHistogram) Record(ctx context.Context, value float64, attrs ...metrics.Attr) {
	h.provider.mu.Lock()
	defer h.provider.mu.Unlock()
	h.provider.histogramCalls++
	h.provider.lastHistogramValue = value
}
