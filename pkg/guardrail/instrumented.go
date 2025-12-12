package guardrail

import (
	"context"
	"time"

	"github.com/hupe1980/agentmesh/pkg/metrics"
)

// Metric names follow AgentMesh conventions.
const (
	MetricChecksTotal   = "agentmesh.guardrail.checks"
	MetricTriggersTotal = "agentmesh.guardrail.triggers"
	MetricDuration      = "agentmesh.guardrail.duration"
	MetricErrorsTotal   = "agentmesh.guardrail.errors"
)

// Layer represents where the guardrail is applied.
type Layer string

// Layer constants define the different levels where guardrails can be applied.
const (
	// LayerAgent applies guardrails at the agent input/output level.
	LayerAgent Layer = "agent"
	// LayerModel applies guardrails at the model middleware level.
	LayerModel Layer = "model"
	// LayerTool applies guardrails at the tool execution level.
	LayerTool Layer = "tool"
)

// InstrumentedGuardrail wraps a guardrail with metrics collection.
// Uses metrics.Provider from context (same pattern as graph/model/tool).
type InstrumentedGuardrail[T any] struct {
	guardrail Guardrail[T]
	layer     Layer
}

// Instrument wraps a guardrail with metrics collection.
func Instrument[T any](g Guardrail[T], layer Layer) Guardrail[T] {
	return &InstrumentedGuardrail[T]{
		guardrail: g,
		layer:     layer,
	}
}

// Name returns the name of the wrapped guardrail.
func (i *InstrumentedGuardrail[T]) Name() string {
	return i.guardrail.Name()
}

// Check validates the input and records metrics.
func (i *InstrumentedGuardrail[T]) Check(ctx context.Context, input T) (*Result, error) {
	start := time.Now()

	result, err := i.guardrail.Check(ctx, input)

	duration := time.Since(start)

	// Get metrics provider from context (existing AgentMesh pattern)
	provider := metrics.FromContext(ctx)

	// Common attributes
	attrs := []metrics.Attr{
		{Key: "guardrail", Value: i.guardrail.Name()},
		{Key: "layer", Value: string(i.layer)},
	}

	if err != nil {
		provider.Counter(MetricErrorsTotal).Add(ctx, 1, attrs...)
		return nil, err
	}

	// Record check with action
	checkAttrs := make([]metrics.Attr, len(attrs)+1)
	copy(checkAttrs, attrs)
	checkAttrs[len(attrs)] = metrics.Attr{Key: "action", Value: result.Action.String()}
	provider.Counter(MetricChecksTotal).Add(ctx, 1, checkAttrs...)
	provider.Histogram(MetricDuration).Record(ctx, duration.Seconds(), attrs...)

	// Record triggers (non-allow results)
	if !result.IsAllowed() {
		provider.Counter(MetricTriggersTotal).Add(ctx, 1, checkAttrs...)
	}

	return result, nil
}

// InstrumentAll wraps multiple guardrails with metrics.
func InstrumentAll[T any](layer Layer, guardrails ...Guardrail[T]) []Guardrail[T] {
	instrumented := make([]Guardrail[T], len(guardrails))
	for idx, g := range guardrails {
		instrumented[idx] = Instrument(g, layer)
	}

	return instrumented
}
