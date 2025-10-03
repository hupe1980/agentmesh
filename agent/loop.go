package agent

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
)

// ErrEscalated is returned when a child agent signals escalation.
var ErrEscalated = errors.New("child agent escalated")

// LoopAgentOptions defines configuration options for LoopAgent behavior.
type LoopAgentOptions struct {
	// Human-readable agent description
	Description string
	// MaxIters specifies the maximum number of iterations to perform.
	MaxIters int
	// Interval specifies the time to wait between iterations.
	Interval time.Duration
	// StopOnError determines whether to stop the loop on errors.
	StopOnError bool
	// Agent executor for running agent tasks
	AgentExecutor core.AgentExecutor
}

// LoopAgent coordinates the repeated execution of a child agent.
//
// This agent type enables iterative workflows by executing a child agent
// multiple times with configurable termination conditions. The loop can
// be controlled by maximum iterations, custom predicates, interval timing,
// and error handling strategies.
//
// Key features:
//   - Configurable maximum iteration limits
//   - Custom termination predicates based on output
//   - Interval timing between iterations
//   - Flexible error handling (stop or continue)
//   - Context cancellation support
//   - Shared session state across iterations
//
// LoopAgent is ideal for:
//   - Monitoring and polling scenarios
//   - Iterative data processing workflows
//   - Retry logic with custom conditions
//   - Periodic task execution
//   - Workflows requiring convergence checking
type LoopAgent struct {
	*BaseAgent
	child         core.Agent         // Child agent to execute repeatedly
	maxIters      int                // Maximum number of iterations allowed
	interval      time.Duration      // Time delay between iterations
	stopOnError   bool               // Whether to stop execution on child agent errors
	agentExecutor core.AgentExecutor // Executor for running agent tasks
}

// DefaultLoopAgentOptions returns the default loop agent configuration.
func DefaultLoopAgentOptions() LoopAgentOptions {
	return LoopAgentOptions{
		Description:   "",
		MaxIters:      100,
		Interval:      0,
		StopOnError:   true,
		AgentExecutor: DefaultAgentExecutor,
	}
}

// NewLoopAgent constructs a looping coordinator around a child agent.
// The child is wired at construction; the hierarchy is read-only at runtime.
func NewLoopAgent(name string, child core.Agent, optFns ...func(o *LoopAgentOptions)) *LoopAgent {
	opts := DefaultLoopAgentOptions()

	for _, fn := range optFns {
		fn(&opts)
	}

	a := &LoopAgent{
		child:         child,
		maxIters:      opts.MaxIters,
		interval:      opts.Interval,
		stopOnError:   opts.StopOnError,
		agentExecutor: opts.AgentExecutor,
	}

	a.BaseAgent = NewBaseAgent(a, name, opts.Description)

	return a
}

// Run executes the child agent repeatedly according to configuration.
//
// This method implements the iterative execution pattern with escalation support:
//  1. Starts the loop agent coordinator
//  2. Executes the child agent up to maxIters times
//  3. Monitors events for escalation signals from child agents
//  4. Checks custom predicate for early termination
//  5. Applies interval delays between iterations
//  6. Handles errors according to stopOnError setting
//  7. Respects context cancellation throughout execution
//  8. Manages cleanup and lifecycle
//
// The same RunContext is passed to all iterations, allowing
// the child agent to accumulate state across loop executions.
//
// If a child agent emits an event with Escalate=true, the loop immediately
// terminates and forwards the escalation event, following the Google ADK pattern.
//
// Parameters:
//   - reqCtx: Shared RequestContext maintained across all iterations
//
// Returns an error if execution fails or if configured to stop on child errors.
// Run implements core.Agent performing iterative execution with escalation
// detection. It returns early (nil error) on escalation events.
func (l *LoopAgent) Run(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error {
	log := logging.FromContext(ctx).With("agent", l.Name())

	// Execute the loop with configured termination conditions and escalation monitoring
	for i := 0; i < l.maxIters; i++ {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Debug("loop.start_iteration", "iter", i+1)

		var escalated atomic.Bool

		intercept := eventWriterFunc(func(c context.Context, ev *core.Event) error {
			if ev.Actions.Escalate.Or(false) {
				escalated.Store(true)
			}

			return queue.Write(c, ev)
		})

		if err := l.agentExecutor.Execute(ctx, reqCtx, l.child, intercept); err != nil {
			if l.stopOnError {
				return fmt.Errorf("loop iteration %d failed for agent %s: %w", i+1, l.child.Name(), err)
			}

			log.Warn("loop.child_error_continue", "iter", i+1, "error", err)
			// Continue loop if configured to ignore errors
		}

		if escalated.Load() {
			return nil
		}

		// Apply interval delay between iterations (except after last iteration)
		if l.interval > 0 && i < l.maxIters-1 {
			log.Debug("loop.sleep", "duration", l.interval)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(l.interval):
				// Continue to next iteration
			}
		}
	}

	log.Info("loop.complete", "iters", l.maxIters)

	return nil
}

// Interface compliance (compile-time assertions)
var _ core.Agent = (*LoopAgent)(nil)
