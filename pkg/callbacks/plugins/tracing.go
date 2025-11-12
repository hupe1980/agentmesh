package plugins

import (
	"context"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// TracingPlugin integrates with distributed tracing systems (OpenTelemetry, Jaeger, etc.)
// to create spans for model calls, tool calls, and graph executions.
type TracingPlugin struct {
	callbacks.NoopPlugin

	tracer trace.Tracer
	spans  sync.Map // map[string]trace.Span for active spans
}

// NewTracingPlugin creates a tracing plugin with the given tracer.
func NewTracingPlugin(tracer trace.Tracer) *TracingPlugin {
	return &TracingPlugin{
		tracer: tracer,
	}
}

// BeforeModel starts a distributed tracing span for model invocation.
func (p *TracingPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	_, span := p.tracer.Start(ctx, "model.call",
		trace.Attr{Key: "messages", Value: len(req.Messages)},
		trace.Attr{Key: "has_tools", Value: len(req.Tools) > 0},
	)

	p.spans.Store("model", span)
	return nil, nil
}

// AfterModel completes the model invocation span with success.
func (p *TracingPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	if spanVal, ok := p.spans.LoadAndDelete("model"); ok {
		span := spanVal.(trace.Span)
		span.End(nil)
	}

	return nil, nil
}

// OnModelError completes the model invocation span with error.
func (p *TracingPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	if spanVal, ok := p.spans.LoadAndDelete("model"); ok {
		span := spanVal.(trace.Span)
		span.End(err)
	}

	return nil, err
}

// BeforeTool starts a distributed tracing span for tool execution.
func (p *TracingPlugin) BeforeTool(ctx context.Context, toolName string, input any) error {
	_, span := p.tracer.Start(ctx, fmt.Sprintf("tool.%s", toolName),
		trace.Attr{Key: "tool", Value: toolName},
	)

	p.spans.Store(fmt.Sprintf("tool:%s", toolName), span)
	return nil
}

// AfterTool completes the tool execution span with success.
func (p *TracingPlugin) AfterTool(ctx context.Context, toolName string, result callbacks.ToolResult) error {
	spanKey := fmt.Sprintf("tool:%s", toolName)
	if spanVal, ok := p.spans.LoadAndDelete(spanKey); ok {
		span := spanVal.(trace.Span)
		span.End(nil)
	}

	return nil
}

// OnToolError completes the tool execution span with error.
func (p *TracingPlugin) OnToolError(ctx context.Context, toolName string, err error) error {
	spanKey := fmt.Sprintf("tool:%s", toolName)
	if spanVal, ok := p.spans.LoadAndDelete(spanKey); ok {
		span := spanVal.(trace.Span)
		span.End(err)
	}

	return nil
}

// OnGraphStart starts a distributed tracing span for graph execution.
func (p *TracingPlugin) OnGraphStart(ctx context.Context, graphID string) error {
	_, span := p.tracer.Start(ctx, "graph.execute",
		trace.Attr{Key: "graph_id", Value: graphID},
	)

	p.spans.Store(fmt.Sprintf("graph:%s", graphID), span)
	return nil
}

// OnGraphComplete completes the graph execution span with statistics.
func (p *TracingPlugin) OnGraphComplete(ctx context.Context, graphID string, stats callbacks.GraphStats) error {
	spanKey := fmt.Sprintf("graph:%s", graphID)
	if spanVal, ok := p.spans.LoadAndDelete(spanKey); ok {
		span := spanVal.(trace.Span)
		span.End(nil)
	}

	return nil
}

// OnGraphError completes the graph execution span with error.
func (p *TracingPlugin) OnGraphError(ctx context.Context, graphID string, err error) error {
	spanKey := fmt.Sprintf("graph:%s", graphID)
	if spanVal, ok := p.spans.LoadAndDelete(spanKey); ok {
		span := spanVal.(trace.Span)
		span.End(err)
	}

	return nil
}
