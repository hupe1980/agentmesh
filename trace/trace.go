package trace

import "context"

// Attr represents a key-value pair for trace attributes.
type Attr struct {
	// Key is the attribute key.
	Key string
	// Value is the attribute value.
	Value any
}

// Span represents a single unit of work in a trace.
type Span interface {
	// End marks the end of the span.
	End(err error)
}

// Tracer represents a mechanism for creating and managing spans.
type Tracer interface {
	// Start begins a new span.
	Start(ctx context.Context, name string, attrs ...Attr) (context.Context, Span)
}

// Provider represents a trace provider.
type Provider interface {
	// Tracer returns a Tracer for the given name.
	Tracer(name string) Tracer
}

// ---- Context helpers for tracer provider propagation ----

// tracerCtxKey is a unique key type for context storage.
type tracerCtxKey struct{}

var _tracerKey = tracerCtxKey{}

// WithProvider returns a child context carrying the tracer provider (defaults to Noop).
func WithProvider(ctx context.Context, tp Provider) context.Context {
	if tp == nil {
		tp = Noop()
	}
	return context.WithValue(ctx, _tracerKey, tp)
}

// FromContext retrieves a tracer provider from context or a no-op.
func FromContext(ctx context.Context) Provider {
	if v := ctx.Value(_tracerKey); v != nil {
		if tp, ok := v.(Provider); ok && tp != nil {
			return tp
		}
	}
	return Noop()
}
