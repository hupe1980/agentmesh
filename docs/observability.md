---
layout: doc
title: Observability
description: Wire structured logging, metrics, and tracing through the AgentMesh runner and use contextual helpers inside your agents and tools.
permalink: /observability/
hero:
  title: Instrument AgentMesh with your observability stack
  description: Configure logging, metrics, and tracing providers once on the runner and consume them everywhere via context helpers.
  primary_cta:
    label: Wire observability
    href: "#configure-the-runner"
  secondary_cta:
    label: Examples →
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples/opentelemetry"
    external: true
sidebar:
  - title: Configure the runner
    url: "#configure-the-runner"
  - title: Logging
    url: "#logging"
  - title: Metrics
    url: "#metrics"
  - title: Tracing
    url: "#tracing"
  - title: Troubleshooting
    url: "#troubleshooting"
---

## Configure the runner {#configure-the-runner}

Observability flows from the runner. Provide your logging, metrics, and tracing implementations via the façade options so every run, agent, and tool invocation receives the same contextual providers.

```go
import (
  am "github.com/hupe1980/agentmesh"
  "github.com/hupe1980/agentmesh/logging"
  metricsotel "github.com/hupe1980/agentmesh/metrics/opentelemetry"
  "github.com/hupe1980/agentmesh/model/openai"
  "github.com/hupe1980/agentmesh/tool"
  traceotel "github.com/hupe1980/agentmesh/trace/opentelemetry"
)

model := openai.NewModel()
agent, err := am.NewModelAgent("instrumented", model, func(o *am.ModelAgentOptions) {
  o.Tools = []tool.Tool{tool.NewFuncTool(...)}
})
if err != nil {
  panic(err)
}

logger := logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatJSON, true)
tp, mp, _ := initOTel() // set up OpenTelemetry exporters elsewhere

application := am.NewApp("instrumented_app", agent)
runner := am.NewRunner(application, func(o *am.RunnerOptions) {
  o.Logger = logger
  o.Metrics = metricsotel.New(mp)
  o.Tracer = traceotel.New(tp)
})
```

Any agent or tool executed by this runner can retrieve the configured providers from the context. If you omit a provider, AgentMesh falls back to no-op implementations so instrumentation calls remain safe.

---

## Logging {#logging}

Use `logging.FromContext(ctx)` inside your agents and tools to retrieve the structured logger that the runner injected. The logger implements `logging.Logger` (slog-compatible) so you can emit JSON events enriched with run metadata.

```go
import "github.com/hupe1980/agentmesh/logging"

func (a *AuditAgent) Run(ctx context.Context, req core.RequestContext, q core.EventWriter) error {
  log := logging.FromContext(ctx)
  log.Info("agent.start", "agent", a.Name(), "session_id", req.SessionID())
  // ... do work ...
  log.Info("agent.finish", "agent", a.Name(), "run_id", req.RunID())
  return nil
}
```

The logger automatically receives attributes such as `run_id` and `application` from the runner so downstream systems can correlate events.

---

## Metrics {#metrics}

Metrics providers implement the `metrics.Provider` interface. Use the contextual helper to record counters, gauges, and histograms without managing exporter plumbing in business logic.

```go
import "github.com/hupe1980/agentmesh/metrics"

func (a *BillingAgent) Run(ctx context.Context, req core.RequestContext, q core.EventWriter) error {
  metrics.FromContext(ctx).Counter("agentmesh_runs_total").Add(ctx, 1,
    metrics.Attr{Key: "agent", Value: a.Name()},
  )
  // ... do work ...
  return nil
}
```

The OpenTelemetry adapter (`metrics/opentelemetry`) bridges AgentMesh metrics to any OTLP backend. Swap it with your own implementation if you prefer Prometheus, StatsD, or another collector.

---

## Tracing {#tracing}

Tracing hooks connect spans around every run, agent, and tool invocation. Retrieve the provider via `trace.FromContext(ctx)` to start spans within your custom code.

```go
import "github.com/hupe1980/agentmesh/trace"

func (a *ChainAgent) Run(ctx context.Context, req core.RequestContext, q core.EventWriter) error {
  tracer := trace.FromContext(ctx).Tracer("agentmesh/chain-agent")
  ctx, span := tracer.Start(ctx, "ChainAgent.Run")
  defer span.End(nil)

  // ... do work ...
  return nil
}
```

The default `trace.Noop()` provider avoids instrumentation errors when tracing is disabled. When you pass `traceotel.New(tp)` from OpenTelemetry, AgentMesh automatically wires parent/child spans so each run appears as a trace with nested agent/tool nodes.

---

## Troubleshooting {#troubleshooting}

- **Seeing no logs/metrics/spans?** Verify the runner received non-nil providers and that your exporter flushes before shutdown. The OpenTelemetry example calls `ForceFlush` / `Shutdown` in `defer` blocks.
- **Attributes missing?** Ensure you propagate the `ctx` returned from AgentMesh helpers into your subcalls. Dropping the context chain strips instrumentation metadata.
- **Custom providers**: implement the relevant interfaces (`logging.Logger`, `metrics.Provider`, `trace.Provider`) and inject them through `am.RunnerOptions`—AgentMesh does not enforce OpenTelemetry.

With observability configured, the `examples/opentelemetry` project shows a full end-to-end setup including stdout exporters and structured logging.
