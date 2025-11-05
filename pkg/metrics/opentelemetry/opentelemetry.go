package opentelemetry

import (
	"context"
	"fmt"

	apimetrics "github.com/hupe1980/agentmesh/pkg/metrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// Provider is an OpenTelemetry-backed implementation of agentmesh metrics.Provider.
type Provider struct {
	meter otelmetric.Meter
}

// New constructs a new OpenTelemetry metrics Provider. If mp is nil, the global
// otel MeterProvider is used.
func New(mp otelmetric.MeterProvider) apimetrics.Provider {
	if mp == nil {
		mp = otel.GetMeterProvider()
	}

	return &Provider{meter: mp.Meter("agentmesh/metrics")}
}

// Counter returns a Float64Counter backed by OpenTelemetry. On instrument
// creation failure, a no-op counter is returned.
func (p *Provider) Counter(name string) apimetrics.Counter {
	inst, err := p.meter.Float64Counter(name)
	if err != nil {
		return noopCounter{}
	}

	return &counter{inst: inst}
}

// Histogram returns a Float64Histogram backed by OpenTelemetry. On instrument
// creation failure, a no-op histogram is returned.
func (p *Provider) Histogram(name string) apimetrics.Histogram {
	inst, err := p.meter.Float64Histogram(name)
	if err != nil {
		return noopHistogram{}
	}

	return &histogram{inst: inst}
}

type counter struct{ inst otelmetric.Float64Counter }

func (c *counter) Add(ctx context.Context, value float64, attrs ...apimetrics.Attr) {
	if c == nil || c.inst == nil {
		return
	}

	kvs := kvsFrom(attrs)
	c.inst.Add(ctx, value, otelmetric.WithAttributes(kvs...))
}

type histogram struct{ inst otelmetric.Float64Histogram }

func (h *histogram) Record(ctx context.Context, value float64, attrs ...apimetrics.Attr) {
	if h == nil || h.inst == nil {
		return
	}

	kvs := kvsFrom(attrs)
	h.inst.Record(ctx, value, otelmetric.WithAttributes(kvs...))
}

// kvsFrom converts our Attr slice into OpenTelemetry attribute key/values.
func kvsFrom(attrs []apimetrics.Attr) []attribute.KeyValue {
	if len(attrs) == 0 {
		return nil
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

	return kvs
}

// no-op fallbacks when instrument creation fails
type noopCounter struct{}

func (noopCounter) Add(context.Context, float64, ...apimetrics.Attr) {}

type noopHistogram struct{}

func (noopHistogram) Record(context.Context, float64, ...apimetrics.Attr) {}
