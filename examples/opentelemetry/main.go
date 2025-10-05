package main

import (
	"context"
	"fmt"
	"time"

	am "github.com/hupe1980/agentmesh"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	metricsotel "github.com/hupe1980/agentmesh/metrics/opentelemetry"
	"github.com/hupe1980/agentmesh/trace"
	traceotel "github.com/hupe1980/agentmesh/trace/opentelemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func initOTel() (*sdktrace.TracerProvider, *sdkmetric.MeterProvider, *sdkmetric.PeriodicReader, error) {
	texp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, nil, nil, err
	}
	mexp, err := stdoutmetric.New()
	if err != nil {
		return nil, nil, nil, err
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(texp))
	reader := sdkmetric.NewPeriodicReader(mexp, sdkmetric.WithInterval(200*time.Millisecond))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	return tp, mp, reader, nil
}

func main() {
	ctx := context.Background()
	tp, mp, reader, err := initOTel()
	if err != nil {
		panic(err)
	}
	defer func() {
		// Flush metrics and traces on exit
		_ = reader.ForceFlush(ctx)
		_ = tp.Shutdown(ctx)
	}()

	// Wrap OTel providers with AgentMesh adapters
	tracerProv := traceotel.New(tp)
	metricsProv := metricsotel.New(mp)

	// Create a simple structured logger (slog-based)
	logger := logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatJSON, true)

	// Create a simple functional agent
	agentName := "otel_demo_agent"
	a := am.NewFuncAgent(agentName, func(ctx context.Context, reqCtx core.RequestContext, q core.EventWriter) error {
		tr := trace.FromContext(ctx).Tracer("agentmesh/examples/otel")
		log := logging.FromContext(ctx)
		ctx, span := tr.Start(ctx, "Agent.Run", trace.Attr{Key: "agent", Value: agentName})
		defer span.End(nil)

		// Context-based logging
		log.Info("running", "agent", agentName, "session_id", reqCtx.SessionID())

		// Simulate a bit of work
		time.Sleep(50 * time.Millisecond)

		// Emit a single assistant message
		ev := core.NewFullAssistantEvent(
			reqCtx.RunID(),
			agentName,
			am.NewPartFromText("Hello from OTel example"),
		)
		_ = q.Write(ctx, ev)
		return nil
	})

	application := am.NewApp("otel_demo_app", a)

	r := am.NewRunner(application, func(o *am.RunnerOptions) {
		o.Tracer = tracerProv
		o.Metrics = metricsProv
		o.Logger = logger
	})
	defer r.Close()

	// Run with a simple user message and print events
	runID, results, err := r.Run(ctx, "user1", "sess1", []am.Part{am.NewPartFromText("ping")})
	if err != nil {
		panic(err)
	}
	fmt.Println("RunID:", runID)
	for res := range results {
		if res.Err != nil {
			fmt.Println("error:", res.Err)
			continue
		}
		if res.Event != nil && len(res.Event.Parts) > 0 {
			for _, p := range res.Event.Parts {
				if t, ok := p.(*core.TextPart); ok {
					fmt.Println("assistant:", t.Text)
				}
			}
		}
	}
}
