package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/metrics"
	"github.com/hupe1980/agentmesh/trace"
)

const (
	metricAgentRunsTotal   = "agentmesh_agent_runs_total"
	metricAgentRunDuration = "agentmesh_agent_run_duration_seconds"
	traceAgentNamespace    = "agentmesh/agent"
)

// ExecuteAgent runs an agent with BeforeAgent / AfterAgent hook semantics.
//
// Lifecycle:
//  1. BeforeAgent: if it returns a non-nil []Part, the agent's Run is skipped and
//     those parts are emitted as a synthetic assistant event (short-circuit). AfterAgent still runs.
//  2. Agent Run (only if not short-circuited) emits its normal events directly to the provided writer.
//  3. AfterAgent: if it returns a non-nil []Part, a new assistant event is appended
//     (it does not mutate or retract earlier output).
//
// History is strictly append-only; no prior events are modified or removed.
func ExecuteAgent(ctx context.Context, reqCtx core.RequestContext, ag core.Agent, w core.EventWriter) error {
	tr := trace.FromContext(ctx).Tracer(traceAgentNamespace)
	met := metrics.FromContext(ctx)
	log := logging.FromContext(ctx).With("agent", ag.Name())

	ctx, span := tr.Start(ctx, "Agent.Execute",
		trace.Attr{Key: "agent.name", Value: ag.Name()},
		trace.Attr{Key: "run.id", Value: reqCtx.RunID()},
	)

	start := time.Now()

	defer func() {
		met.Histogram(metricAgentRunDuration).Record(
			ctx,
			time.Since(start).Seconds(),
			metrics.Attr{Key: "agent.name", Value: ag.Name()},
		)
		span.End(nil)
	}()

	met.Counter(metricAgentRunsTotal).Add(ctx, 1, metrics.Attr{Key: "agent.name", Value: ag.Name()})

	log.Info("agent.execute.start")

	// If the RequestContext's agent identity doesn't match the target agent's name,
	// clone the context so emitted events have the correct Author. This centralizes
	// transfer / delegation behavior so callers don't need to clone manually.
	if reqCtx.AgentName() != ag.Name() { // lightweight check; cloning is cheap (shallow)
		reqCtx = core.CloneRequestContextWithAgent(reqCtx, ag)
	}

	// BeforeAgent short-circuit path
	if parts, err := reqCtx.RunBeforeAgent(ctx, ag); err != nil {
		log.Error("agent.before.error", "error", err)

		return fmt.Errorf("plugin: before_agent: %w", err)
	} else if parts != nil {
		assist := core.NewFullAssistantEvent(reqCtx.RunID(), reqCtx.AgentName(), parts...)

		if err := w.Write(ctx, assist); err != nil {
			log.Error("agent.synthetic.write.error", "error", err)
			return fmt.Errorf("failed to write synthetic assistant event: %w", err)
		}

		log.Info("agent.execute.short_circuit")

		return runAfterAgent(ctx, reqCtx, ag, w)
	}

	if err := ag.Run(ctx, reqCtx, w); err != nil {
		log.Error("agent.run.error", "error", err)
		return err
	}

	log.Info("agent.run.finished")

	return runAfterAgent(ctx, reqCtx, ag, w)
}

// runAfterAgent invokes the AfterAgent plugin hook and, if parts are returned,
// appends a new assistant event. Returns any error encountered.
func runAfterAgent(ctx context.Context, reqCtx core.RequestContext, ag core.Agent, w core.EventWriter) error {
	if afterParts, err := reqCtx.RunAfterAgent(ctx, ag); err != nil {
		return fmt.Errorf("plugin: after_agent: %w", err)
	} else if afterParts != nil {
		repl := core.NewFullAssistantEvent(reqCtx.RunID(), reqCtx.AgentName(), afterParts...)

		if err := w.Write(ctx, repl); err != nil {
			return fmt.Errorf("failed to write after_agent replacement event: %w", err)
		}
	}

	return nil
}

// DefaultAgentExecutor is the reusable core.AgentExecutor implementation
// using ExecuteAgent. Inject this into flows/selectors.
var DefaultAgentExecutor core.AgentExecutor = core.AgentExecutorFunc(ExecuteAgent)

// Compile-time assertion for the function adapter variable.
var _ core.AgentExecutor = DefaultAgentExecutor
