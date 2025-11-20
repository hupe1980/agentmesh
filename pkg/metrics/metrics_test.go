package metrics_test

import (
	"context"
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoopProvider verifies the no-op implementation doesn't panic
func TestNoopProvider(t *testing.T) {
	ctx := context.Background()
	provider := metrics.Noop()

	counter := provider.Counter("test_counter")
	histogram := provider.Histogram("test_histogram")

	// Should not panic
	assert.NotPanics(t, func() {
		counter.Add(ctx, 1.0)
		counter.Add(ctx, 5.0, metrics.Attr{Key: "label", Value: "test"})
		histogram.Record(ctx, 100.0)
		histogram.Record(ctx, 200.0, metrics.Attr{Key: "bucket", Value: "slow"})
	})
}

// TestContextPropagation verifies provider is stored and retrieved from context
func TestContextPropagation(t *testing.T) {
	ctx := context.Background()

	// Without provider, should get no-op
	provider1 := metrics.FromContext(ctx)
	require.NotNil(t, provider1, "FromContext should never return nil")

	// With provider using local mock
	mockProvider := &mockMetricsProvider{}
	ctx = metrics.WithProvider(ctx, mockProvider)

	provider2 := metrics.FromContext(ctx)
	assert.Equal(t, mockProvider, provider2, "Should retrieve the same provider from context")
}

// TestNilProviderDefaults verifies nil provider defaults to no-op
func TestNilProviderDefaults(t *testing.T) {
	ctx := context.Background()
	ctx = metrics.WithProvider(ctx, nil)

	provider := metrics.FromContext(ctx)
	require.NotNil(t, provider, "WithProvider(nil) should default to no-op provider")

	// Should not panic
	counter := provider.Counter("test")
	assert.NotPanics(t, func() {
		counter.Add(ctx, 1.0)
	})
}

// TestConcurrentAccess verifies thread safety of context operations
func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	mockProvider := &mockMetricsProvider{}
	ctx = metrics.WithProvider(ctx, mockProvider)

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

	mockProvider.mu.Lock()
	calls := mockProvider.counterCalls
	mockProvider.mu.Unlock()

	assert.Equal(t, 100, calls, "Expected 100 counter calls")
}

// TestAttrCreation verifies Attr struct construction
func TestAttrCreation(t *testing.T) {
	attr := metrics.Attr{Key: "environment", Value: "production"}

	assert.Equal(t, "environment", attr.Key)
	assert.Equal(t, "production", attr.Value)
}

// TestMultipleAttributes verifies multiple attributes can be passed
func TestMultipleAttributes(t *testing.T) {
	ctx := context.Background()
	mockProvider := &mockMetricsProvider{}

	counter := mockProvider.Counter("test")
	counter.Add(ctx, 1.0,
		metrics.Attr{Key: "region", Value: "us-east-1"},
		metrics.Attr{Key: "service", Value: "api"},
	)

	mockProvider.mu.Lock()
	calls := mockProvider.counterCalls
	attrs := mockProvider.lastCounterAttrs
	mockProvider.mu.Unlock()

	assert.Equal(t, 1, calls)
	require.Len(t, attrs, 2)
	assert.Equal(t, "region", attrs[0].Key)
	assert.Equal(t, "us-east-1", attrs[0].Value)
	assert.Equal(t, "service", attrs[1].Key)
	assert.Equal(t, "api", attrs[1].Value)
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
