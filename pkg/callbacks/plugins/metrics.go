package plugins

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// MetricsPlugin collects execution metrics for observability.
// It tracks model calls, tool calls, errors, latencies, and graph executions.
type MetricsPlugin struct {
	callbacks.NoopPlugin

	provider metrics.Provider

	// Atomic counters for thread-safe incrementing
	modelCalls      atomic.Int64
	modelErrors     atomic.Int64
	toolCalls       atomic.Int64
	toolErrors      atomic.Int64
	graphExecutions atomic.Int64
	graphErrors     atomic.Int64
	messagesEmitted atomic.Int64

	// Latency tracking with mutex protection
	mu           sync.Mutex
	modelLatency []time.Duration
	toolLatency  []time.Duration

	// Timing state for measuring durations
	timers sync.Map // map[string]time.Time
}

// NewMetricsPlugin creates a metrics collection plugin.
// If provider is nil, metrics are only tracked internally.
func NewMetricsPlugin(provider metrics.Provider) *MetricsPlugin {
	return &MetricsPlugin{
		provider:     provider,
		modelLatency: []time.Duration{},
		toolLatency:  []time.Duration{},
	}
}

// Init registers metrics with the provider.
func (p *MetricsPlugin) Init(ctx context.Context) error {
	// Register metrics with provider if available
	if p.provider != nil {
		p.provider.Counter("agentmesh.model.calls")
		p.provider.Counter("agentmesh.model.errors")
		p.provider.Counter("agentmesh.tool.calls")
		p.provider.Counter("agentmesh.tool.errors")
		p.provider.Counter("agentmesh.graph.executions")
		p.provider.Counter("agentmesh.graph.errors")
		p.provider.Counter("agentmesh.messages.emitted")
		p.provider.Histogram("agentmesh.model.latency")
		p.provider.Histogram("agentmesh.tool.latency")
		p.provider.Histogram("agentmesh.graph.duration")
	}
	return nil
}

// BeforeModel records model invocation start time.
func (p *MetricsPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	p.modelCalls.Add(1)
	p.timers.Store("model", time.Now())

	if p.provider != nil {
		p.provider.Counter("agentmesh.model.calls").Add(ctx, 1)
	}

	return nil, nil
}

// AfterModel records model invocation latency.
func (p *MetricsPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	if start, ok := p.timers.LoadAndDelete("model"); ok {
		latency := time.Since(start.(time.Time))

		p.mu.Lock()
		p.modelLatency = append(p.modelLatency, latency)
		p.mu.Unlock()

		if p.provider != nil {
			p.provider.Histogram("agentmesh.model.latency").Record(ctx, float64(latency.Milliseconds()))
		}
	}

	return nil, nil
}

// OnModelError increments model error counter.
func (p *MetricsPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	p.modelErrors.Add(1)

	if p.provider != nil {
		p.provider.Counter("agentmesh.model.errors").Add(ctx, 1,
			metrics.Attr{Key: "error", Value: err.Error()})
	}

	return nil, err
}

// BeforeTool records tool execution start time.
func (p *MetricsPlugin) BeforeTool(ctx context.Context, toolName string, input any) error {
	p.toolCalls.Add(1)
	p.timers.Store(fmt.Sprintf("tool:%s", toolName), time.Now())

	if p.provider != nil {
		p.provider.Counter("agentmesh.tool.calls").Add(ctx, 1,
			metrics.Attr{Key: "tool", Value: toolName})
	}

	return nil
}

// AfterTool records tool execution latency.
func (p *MetricsPlugin) AfterTool(ctx context.Context, toolName string, result callbacks.ToolResult) error {
	timerKey := fmt.Sprintf("tool:%s", toolName)
	if start, ok := p.timers.LoadAndDelete(timerKey); ok {
		latency := time.Since(start.(time.Time))

		p.mu.Lock()
		p.toolLatency = append(p.toolLatency, latency)
		p.mu.Unlock()

		if p.provider != nil {
			p.provider.Histogram("agentmesh.tool.latency").Record(ctx, float64(latency.Milliseconds()),
				metrics.Attr{Key: "tool", Value: toolName})
		}
	}

	return nil
}

// OnToolError increments tool error counter.
func (p *MetricsPlugin) OnToolError(ctx context.Context, toolName string, err error) error {
	p.toolErrors.Add(1)

	if p.provider != nil {
		p.provider.Counter("agentmesh.tool.errors").Add(ctx, 1,
			metrics.Attr{Key: "tool", Value: toolName},
			metrics.Attr{Key: "error", Value: err.Error()})
	}

	return nil
}

// OnGraphStart increments graph execution counter.
func (p *MetricsPlugin) OnGraphStart(ctx context.Context, graphID string) error {
	p.graphExecutions.Add(1)
	p.timers.Store(fmt.Sprintf("graph:%s", graphID), time.Now())

	if p.provider != nil {
		p.provider.Counter("agentmesh.graph.executions").Add(ctx, 1)
	}

	return nil
}

// OnGraphComplete records graph execution metrics.
func (p *MetricsPlugin) OnGraphComplete(ctx context.Context, graphID string, stats callbacks.GraphStats) error {
	timerKey := fmt.Sprintf("graph:%s", graphID)
	if start, ok := p.timers.LoadAndDelete(timerKey); ok {
		duration := time.Since(start.(time.Time))

		if p.provider != nil {
			p.provider.Histogram("agentmesh.graph.duration").Record(ctx, float64(duration.Milliseconds()),
				metrics.Attr{Key: "nodes_visited", Value: stats.NodesVisited})
		}
	}

	return nil
}

// OnGraphError increments graph error counter.
func (p *MetricsPlugin) OnGraphError(ctx context.Context, graphID string, err error) error {
	p.graphErrors.Add(1)
	p.timers.Delete(fmt.Sprintf("graph:%s", graphID))

	if p.provider != nil {
		p.provider.Counter("agentmesh.graph.errors").Add(ctx, 1,
			metrics.Attr{Key: "error", Value: err.Error()})
	}

	return nil
}

// OnMessage increments message emission counter.
func (p *MetricsPlugin) OnMessage(ctx context.Context, msg message.Message) error {
	p.messagesEmitted.Add(1)

	if p.provider != nil {
		p.provider.Counter("agentmesh.messages.emitted").Add(ctx, 1,
			metrics.Attr{Key: "type", Value: string(msg.Type())})
	}

	return nil
}

// GetSnapshot returns a snapshot of current metrics.
func (p *MetricsPlugin) GetSnapshot() MetricsSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	return MetricsSnapshot{
		ModelCalls:      p.modelCalls.Load(),
		ModelErrors:     p.modelErrors.Load(),
		ToolCalls:       p.toolCalls.Load(),
		ToolErrors:      p.toolErrors.Load(),
		GraphExecutions: p.graphExecutions.Load(),
		GraphErrors:     p.graphErrors.Load(),
		MessagesEmitted: p.messagesEmitted.Load(),
		AvgModelLatency: average(p.modelLatency),
		AvgToolLatency:  average(p.toolLatency),
	}
}

// MetricsSnapshot contains a point-in-time view of all metrics.
type MetricsSnapshot struct {
	ModelCalls      int64
	ModelErrors     int64
	ToolCalls       int64
	ToolErrors      int64
	GraphExecutions int64
	GraphErrors     int64
	MessagesEmitted int64
	AvgModelLatency time.Duration
	AvgToolLatency  time.Duration
}

func average(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}
