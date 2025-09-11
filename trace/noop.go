package trace

import "context"

// Noop is a no-op implementation of the trace.Provider interface.
func Noop() Provider { return noopProvider{} }

type noopProvider struct{}

func (noopProvider) Tracer(string) Tracer { return noopTracer{} }

type noopTracer struct{}

func (noopTracer) Start(ctx context.Context, _ string, _ ...Attr) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End(error) {}
