package opentelemetry

import (
	"context"
	"fmt"

	apitrace "github.com/hupe1980/agentmesh/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Provider is an OpenTelemetry-backed implementation of agentmesh trace.Provider.
type Provider struct {
	tp oteltrace.TracerProvider
}

// New constructs a new OpenTelemetry Provider. If tp is nil, the global otel
// TracerProvider is used.
func New(tp oteltrace.TracerProvider) apitrace.Provider {
	if tp == nil {
		tp = otel.GetTracerProvider()
	}

	return &Provider{tp: tp}
}

// Tracer returns a Tracer bound to the given instrumentation name.
func (p *Provider) Tracer(name string) apitrace.Tracer {
	return &tracer{t: p.tp.Tracer(name)}
}

type tracer struct{ t oteltrace.Tracer }

// Start begins a span with optional attributes and returns the derived context
// and a Span adapter implementing the agentmesh trace.Span interface.
func (t *tracer) Start(ctx context.Context, name string, attrs ...apitrace.Attr) (context.Context, apitrace.Span) {
	if t == nil || t.t == nil {
		// Fallback to global if uninitialized
		tp := otel.GetTracerProvider()
		t = &tracer{t: tp.Tracer(name)}
	}

	kvs := make([]attribute.KeyValue, 0, len(attrs))
	for _, a := range attrs {
		switch v := a.Value.(type) {
		case string:
			kvs = append(kvs, attribute.String(a.Key, v))
		case bool:
			kvs = append(kvs, attribute.Bool(a.Key, v))
		case int:
			kvs = append(kvs, attribute.Int(a.Key, v))
		case int64:
			kvs = append(kvs, attribute.Int64(a.Key, v))
		case float64:
			kvs = append(kvs, attribute.Float64(a.Key, v))
		default:
			kvs = append(kvs, attribute.String(a.Key, fmt.Sprint(v)))
		}
	}

	ctx2, sp := t.t.Start(ctx, name, oteltrace.WithAttributes(kvs...))

	return ctx2, &span{sp: sp}
}

type span struct{ sp oteltrace.Span }

// End finalizes the span and records error status if err is non-nil.
func (s *span) End(err error) {
	if s == nil || s.sp == nil {
		return
	}

	if err != nil {
		s.sp.RecordError(err)
		s.sp.SetStatus(codes.Error, err.Error())
	} else {
		s.sp.SetStatus(codes.Ok, "")
	}

	s.sp.End()
}
