package metrics

import "context"

// Attr represents a key-value pair for metric attributes.
type Attr struct {
	// Key is the attribute key.
	Key string
	// Value is the attribute value.
	Value any
}

// Counter represents a metric that counts occurrences.
type Counter interface {
	Add(ctx context.Context, value float64, attrs ...Attr)
}

// Histogram represents a metric that observes the distribution of values.
type Histogram interface {
	Record(ctx context.Context, value float64, attrs ...Attr)
}

// Provider represents a metric provider.
type Provider interface {
	Counter(name string) Counter
	Histogram(name string) Histogram
}

// ---- Context helpers for metrics provider propagation ----

// metricsCtxKey is a unique key type for context value storage.
type metricsCtxKey struct{}

var _metricsKey = metricsCtxKey{}

// WithProvider returns a child context carrying the metrics provider (defaults to Noop).
func WithProvider(ctx context.Context, m Provider) context.Context {
	if m == nil {
		m = Noop()
	}
	return context.WithValue(ctx, _metricsKey, m)
}

// FromContext retrieves a metrics provider from context or a no-op.
func FromContext(ctx context.Context) Provider {
	if v := ctx.Value(_metricsKey); v != nil {
		if m, ok := v.(Provider); ok && m != nil {
			return m
		}
	}
	return Noop()
}
