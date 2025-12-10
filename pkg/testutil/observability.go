package testutil

import (
	"context"
	"sync"
)

// MockMetricsProvider is a mock implementation of metrics.Provider for testing.
type MockMetricsProvider struct {
	mu         sync.Mutex
	Counters   map[string]float64
	Histograms map[string][]float64
}

// NewMockMetricsProvider creates a new MockMetricsProvider.
func NewMockMetricsProvider() *MockMetricsProvider {
	return &MockMetricsProvider{
		Counters:   make(map[string]float64),
		Histograms: make(map[string][]float64),
	}
}

// Counter returns a mock counter that tracks operations.
func (m *MockMetricsProvider) Counter(name string) *MockCounter {
	return &MockCounter{provider: m, name: name}
}

// Histogram returns a mock histogram that tracks operations.
func (m *MockMetricsProvider) Histogram(name string) *MockHistogram {
	return &MockHistogram{provider: m, name: name}
}

// GetCounterValue returns the total value added to a specific counter.
func (m *MockMetricsProvider) GetCounterValue(name string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Counters[name]
}

// GetHistogramValues returns all values recorded to a specific histogram.
func (m *MockMetricsProvider) GetHistogramValues(name string) []float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	values := m.Histograms[name]
	result := make([]float64, len(values))
	copy(result, values)
	return result
}

// Reset clears all recorded metrics.
func (m *MockMetricsProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Counters = make(map[string]float64)
	m.Histograms = make(map[string][]float64)
}

// MockCounter is a mock counter for testing.
type MockCounter struct {
	provider *MockMetricsProvider
	name     string
}

// Add adds a value to the counter.
func (c *MockCounter) Add(ctx context.Context, value float64, attrs ...any) {
	c.provider.mu.Lock()
	defer c.provider.mu.Unlock()
	c.provider.Counters[c.name] += value
}

// MockHistogram is a mock histogram for testing.
type MockHistogram struct {
	provider *MockMetricsProvider
	name     string
}

// Record records a value in the histogram.
func (h *MockHistogram) Record(ctx context.Context, value float64, attrs ...any) {
	h.provider.mu.Lock()
	defer h.provider.mu.Unlock()
	h.provider.Histograms[h.name] = append(h.provider.Histograms[h.name], value)
}

// MockTraceProvider is a mock implementation of trace.Provider for testing.
type MockTraceProvider struct {
	mu    sync.Mutex
	Spans map[string]*MockSpan
}

// NewMockTraceProvider creates a new MockTraceProvider.
func NewMockTraceProvider() *MockTraceProvider {
	return &MockTraceProvider{
		Spans: make(map[string]*MockSpan),
	}
}

// Tracer returns a mock tracer that tracks operations.
func (m *MockTraceProvider) Tracer(name string) *MockTracer {
	return &MockTracer{provider: m}
}

// GetSpan returns a mock span by name.
func (m *MockTraceProvider) GetSpan(name string) *MockSpan {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Spans[name]
}

// SpanCount returns the number of spans created.
func (m *MockTraceProvider) SpanCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Spans)
}

// Reset clears all recorded spans.
func (m *MockTraceProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Spans = make(map[string]*MockSpan)
}

// MockTracer is a mock tracer for testing.
type MockTracer struct {
	provider *MockTraceProvider
}

// Start creates a new mock span.
func (t *MockTracer) Start(ctx context.Context, name string, attrs ...any) (context.Context, *MockSpan) {
	t.provider.mu.Lock()
	defer t.provider.mu.Unlock()

	span := &MockSpan{
		Name: name,
	}
	t.provider.Spans[name] = span

	type spanKey struct{}
	return context.WithValue(ctx, spanKey{}, span), span
}

// MockSpan is a mock span for testing.
type MockSpan struct {
	Name  string
	Ended bool
	Error error
}

// End marks the span as complete.
func (s *MockSpan) End(err error) {
	s.Ended = true
	s.Error = err
}
