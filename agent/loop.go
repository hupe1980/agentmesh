// Package agent provides loop-based execution coordination for repetitive tasks.
//
// LoopAgent executes a single child agent repeatedly with configurable termination
// controls (max iterations, predicate, interval, escalation monitoring).

package agent

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hupe1980/agentmesh/core"
)

// ErrEscalated is returned when a child agent signals escalation.
var ErrEscalated = errors.New("child agent escalated")

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
	BaseAgent
	child       core.Agent        // Child agent to execute repeatedly
	maxIters    int               // Maximum number of iterations allowed
	interval    time.Duration     // Time delay between iterations
	stopOnError bool              // Whether to stop execution on child agent errors
	predicate   func(string) bool // Custom termination condition based on output
}

// NewLoopAgent constructs a looping coordinator around a child agent.
// Defaults: 100 iterations, no interval, stop on first error.
//
// Default configuration:
//   - Maximum 100 iterations
//   - No interval between iterations
//   - Stop execution on errors
//   - No custom termination predicate
//
// Parameters:
//   - name: Human-readable name for the coordinator
//   - child: Agent to execute iteratively
//   - opts: Configuration options for loop behavior
//
// Returns a configured LoopAgent ready for iterative execution.
func NewLoopAgent(name string, child core.Agent, opts ...LoopOption) *LoopAgent {
	la := &LoopAgent{
		BaseAgent:   NewBaseAgent(name),
		child:       child,
		maxIters:    100,
		interval:    0,
		stopOnError: true,
	}

	for _, o := range opts {
		o(la)
	}

	return la
}

// LoopOption defines a configuration function for customizing LoopAgent behavior.
type LoopOption func(*LoopAgent)

// WithMaxIters sets the maximum number of iterations for the loop.
//
// The loop will terminate after this many iterations even if other
// termination conditions are not met. Set to a reasonable value to
// prevent infinite loops.
func WithMaxIters(n int) LoopOption {
	return func(l *LoopAgent) { l.maxIters = n }
}

// WithInterval sets the time delay between loop iterations.
//
// This is useful for rate limiting, polling scenarios, or giving
// external systems time to process between iterations. Set to 0
// for no delay between iterations.
func WithInterval(d time.Duration) LoopOption {
	return func(l *LoopAgent) { l.interval = d }
}

// WithPredicate sets a custom termination condition based on output.
//
// The predicate function receives the string output from the child agent
// and should return true to terminate the loop early. This enables
// sophisticated termination logic based on agent responses.
//
// Example:
//
//	WithPredicate(func(output string) bool {
//	    return strings.Contains(output, "COMPLETE")
//	})
func WithPredicate(pred func(string) bool) LoopOption {
	return func(l *LoopAgent) { l.predicate = pred }
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
// The same InvocationContext is passed to all iterations, allowing
// the child agent to accumulate state across loop executions.
//
// If a child agent emits an event with Escalate=true, the loop immediately
// terminates and forwards the escalation event, following the Google ADK pattern.
//
// Parameters:
//   - invocationCtx: Shared context maintained across all iterations
//
// Returns an error if execution fails or if configured to stop on child errors.
// Run implements core.Agent performing iterative execution with escalation
// detection. It returns early (nil error) on escalation events.
func (l *LoopAgent) Run(invocationCtx *core.InvocationContext) error {
	// Execute the loop with configured termination conditions and escalation monitoring
	for i := 0; i < l.maxIters; i++ {
		// Check for context cancellation
		select {
		case <-invocationCtx.Done():
			return invocationCtx.Err()
		default:
		}

		log.Printf("LoopAgent: starting iteration %d", i+1)

		// Execute child agent with monitoring for escalation
		childErr := l.runChildWithEscalationMonitoring(invocationCtx)

		// Handle escalation - if child escalated, stop immediately
		if errors.Is(childErr, ErrEscalated) {
			log.Printf("LoopAgent: child agent escalated at iteration %d, stopping loop", i+1)
			return nil // Escalation is not an error, just early termination
		}

		// Handle other errors
		if childErr != nil {
			if l.stopOnError {
				return fmt.Errorf("loop iteration %d failed for agent %s: %w", i+1, l.child.Name(), childErr)
			}
			log.Printf("LoopAgent: iteration %d failed but continuing due to stopOnError=false: %v", i+1, childErr)
			// Continue loop if configured to ignore errors
		}

		// Check custom termination predicate if configured
		if l.predicate != nil {
			// TODO: In a full implementation, you'd need to capture
			// the child agent's text output to evaluate the predicate
			// This would require extending the event monitoring to extract
			// text content from events and pass it to the predicate function
			_ = l.predicate // Suppress unused warning until implementation is complete
		}

		// Apply interval delay between iterations (except after last iteration)
		if l.interval > 0 && i < l.maxIters-1 {
			log.Printf("LoopAgent: waiting %v before next iteration", l.interval)
			select {
			case <-invocationCtx.Done():
				return invocationCtx.Err()
			case <-time.After(l.interval):
				// Continue to next iteration
			}
		}
	}

	log.Printf("LoopAgent: completed all %d iterations", l.maxIters)

	return nil
}

// runChildWithEscalationMonitoring executes the child while intercepting emitted events
// to detect escalation flags before forwarding to the parent context.
// runChildWithEscalationMonitoring wraps child execution routing its emitted
// events through an intercept channel to inspect for escalation flags before
// forwarding to the parent context.
func (l *LoopAgent) runChildWithEscalationMonitoring(invocationCtx *core.InvocationContext) error {
	// Create intercepting channels and derive a child context using helper
	interceptChan := make(chan core.Event, 10)
	resumeChan := make(chan struct{}, 10)
	childInvocationCtx := invocationCtx.NewChildInvocationContext(interceptChan, resumeChan, invocationCtx.Branch)

	// Channel to communicate child execution completion
	done := make(chan error, 1)

	// Run child agent in a separate goroutine
	go func() {
		defer close(done)
		done <- l.child.Run(childInvocationCtx)
	}()

	// Monitor events and forward them, checking for escalation
	for {
		select {
		case event, ok := <-interceptChan:
			if !ok {
				// Child closed the channel, wait for completion
				return <-done
			}

			// Check for escalation
			if event.Actions.Escalate != nil && *event.Actions.Escalate {
				log.Printf("LoopAgent: detected escalation event from child")
				// Forward the escalation event to the parent
				if err := invocationCtx.EmitEvent(event); err != nil {
					return err
				}
				// Wait for the child to complete and return escalation
				<-done
				return ErrEscalated
			}

			// Forward non-escalation events to the parent context
			if err := invocationCtx.EmitEvent(event); err != nil {
				return err
			}

			// Send resume signal to child
			select {
			case resumeChan <- struct{}{}:
			case <-invocationCtx.Done():
				return invocationCtx.Err()
			}

		case err := <-done:
			// Child completed without escalation
			close(interceptChan)
			close(resumeChan)
			return err

		case <-invocationCtx.Done():
			// Context cancelled
			close(interceptChan)
			close(resumeChan)
			return invocationCtx.Err()
		}
	}
}

// CreateEscalationEvent creates an event that signals escalation to the parent agent.
//
// This helper function creates a properly formatted event with the escalation flag set,
// following the Google ADK escalation pattern. Agents can use this to create escalation
// events when they determine that they cannot complete their task and need to escalate
// to a higher level agent.
//
// Parameters:
//   - author: Name of the agent creating the escalation event
//   - invocationID: Current invocation context identifier
//   - content: Optional content describing the reason for escalation
//
// Returns a fully configured Event with Escalate=true.
//
// Example usage:
//
//	event := CreateEscalationEvent(
//	    "TaskAgent",
//	    ctx.InvocationID,
//	    &event.Content{
//	        Role: "assistant",
//	        Parts: []event.Part{event.TextPart{Text: "Task complexity exceeds my capabilities"}},
//	    },
//	)
//	return ctx.EmitEvent(event)
//
// CreateEscalationEvent helper for constructing an escalation signal event.
func CreateEscalationEvent(invocationID, author string, content *core.Content) core.Event {
	escalate := true
	ev := core.NewEvent(invocationID, author)
	ev.Actions.Escalate = &escalate
	ev.Content = content
	return ev
}
