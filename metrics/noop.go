package metrics

import "context"

// Noop is a no-op implementation of the metrics.Provider interface.
func Noop() Provider { return noopProvider{} }

type noopProvider struct{}

// Counter returns a no-op Counter.
func (noopProvider) Counter(string) Counter { return noopCounter{} }

// Histogram returns a no-op Histogram.
func (noopProvider) Histogram(string) Histogram { return noopHistogram{} }

type noopCounter struct{}

// Add is a no-op implementation of the Counter interface.
func (noopCounter) Add(context.Context, float64, ...Attr) {}

type noopHistogram struct{}

// Record is a no-op implementation of the Histogram interface.
func (noopHistogram) Record(context.Context, float64, ...Attr) {}

// Compile-time assertions
var (
	_ Provider  = (*noopProvider)(nil)
	_ Counter   = (*noopCounter)(nil)
	_ Histogram = (*noopHistogram)(nil)
)
