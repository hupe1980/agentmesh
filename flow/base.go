package flow

import (
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/model"
)

// BaseFlow is a minimal single‑agent flow implementation that supports a
// request -> LLM -> (optional tool loop) cycle with pluggable pre/post processors.
type BaseFlow struct {
	agent              FlowAgent
	requestProcessors  []RequestProcessor
	responseProcessors []ResponseProcessor
}

// NewBaseFlow creates a new basic single-agent flow.
func NewBaseFlow(agent FlowAgent) *BaseFlow {
	return &BaseFlow{
		agent:              agent,
		requestProcessors:  []RequestProcessor{},
		responseProcessors: []ResponseProcessor{},
	}
}

// AddRequestProcessor adds a request processor to the flow.
// AddRequestProcessor appends a request processor; order of registration defines execution order.
func (f *BaseFlow) AddRequestProcessor(processor RequestProcessor) {
	f.requestProcessors = append(f.requestProcessors, processor)
}

// AddResponseProcessor adds a response processor to the flow.
// AddResponseProcessor appends a response processor executed after each model chunk.
func (f *BaseFlow) AddResponseProcessor(processor ResponseProcessor) {
	f.responseProcessors = append(f.responseProcessors, processor)
}

// Execute launches the flow asynchronously and returns a channel of Events.
// The channel is closed when a final response is emitted or an unrecoverable
// error occurs. Callers should range over the returned channel.
func (f *BaseFlow) Execute(runCtx *core.RunContext) (<-chan core.Event, error) {
	eventChan := make(chan core.Event, 100)

	go func() {
		defer close(eventChan)

		for {
			last := f.runOnce(runCtx, eventChan)
			if last == nil {
				break
			}
			// If we just emitted a function response, we want another LLM turn
			if len(last.GetFunctionResponses()) > 0 {
				continue
			}
			if last.IsPartial() {
				fmt.Println("WARNING: last event is partial, which is not expected.")
				break
			}
			if last.IsFinalResponse() {
				break
			}
		}
	}()

	return eventChan, nil
}

// emitError converts an internal error to a system Event.
func (f *BaseFlow) emitError(eventChan chan<- core.Event, err error) {
	ev := core.NewEvent("", "system")
	msg := err.Error()
	ev.ErrorMessage = &msg
	eventChan <- ev
}

// runOnce performs one model turn (including any tool executions) and returns
// the last emitted Event (final or intermediate). A nil return signals termination.
func (f *BaseFlow) runOnce(runCtx *core.RunContext, eventChan chan<- core.Event) *core.Event {
	// Refresh session snapshot so request processors see latest conversation (including tool responses)
	if runCtx.SessionService != nil {
		if latest, err := runCtx.SessionService.Get(runCtx.SessionID); err == nil && latest != nil {
			runCtx.Session = latest
		}
	}

	// Create a new model request
	req := new(model.Request)

	// Run request processors
	for _, processor := range f.requestProcessors {
		if err := processor.ProcessRequest(runCtx, req, f.agent); err != nil {
			f.emitError(eventChan, fmt.Errorf("request processor %s failed: %w", processor.Name(), err))
			return nil
		}
	}

	// Build tool definitions
	tools := f.agent.GetTools()
	if len(tools) > 0 {
		toolDefinitions := make([]model.ToolDefinition, 0, len(tools))
		for _, t := range tools {
			toolDefinitions = append(toolDefinitions, model.ToolDefinition{
				Type: "function",
				Function: model.FunctionDefinition{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  t.Parameters(),
				},
			})
		}

		// Add tools to request
		req.Tools = toolDefinitions
	}

	// Execute LLM request
	llm := f.agent.GetLLM()

	respCh, errCh := llm.Generate(runCtx.Context, *req)

	var lastEvent *core.Event

loop:
	for {
		select {
		case resp, ok := <-respCh:
			if !ok {
				break loop
			}

			// Apply response processors
			for _, processor := range f.responseProcessors {
				if err := processor.ProcessResponse(runCtx, &resp, f.agent); err != nil {
					f.emitError(eventChan, fmt.Errorf("response processor %s failed: %w", processor.Name(), err))
					return nil
				}
			}

			// Emit processed event
			ev := core.NewEvent(runCtx.RunID, f.agent.GetName())
			ev.Content = &resp.Content
			ev.Partial = &resp.Partial

			// Mark turn complete if this is a final assistant response with no pending tool calls
			if !resp.Partial && len(ev.GetFunctionCalls()) == 0 {
				complete := true
				ev.TurnComplete = &complete
			}

			lastEvent = &ev

			eventChan <- ev

			// Wait for session persistence (runner sends resume after append)
			if !ev.IsPartial() && runCtx.Resume != nil {
				select {
				case <-runCtx.Context.Done():
					return lastEvent
				case <-runCtx.Resume:
				}
			}

			// Handle function calls
			if fnCalls := ev.GetFunctionCalls(); len(fnCalls) > 0 {
				for _, fnCall := range fnCalls {
					toolCtx := core.NewToolContext(runCtx, fnCall.ID)

					start := time.Now()
					result, err := f.agent.ExecuteTool(toolCtx, fnCall.Name, fnCall.Arguments)
					dur := time.Since(start)

					if runCtx.Logger != nil {
						runCtx.Logger.Info("agent.tool.executed", "agent", f.agent.GetName(), "tool", fnCall.Name, "duration_ms", dur.Milliseconds(), "error", err != nil)
					}

					respEv := core.NewFunctionResponseEvent(f.agent.GetName(), fnCall.ID, fnCall.Name, result, err)

					lastEvent = &respEv

					eventChan <- respEv

					// Wait for session persistence of tool response
					if runCtx.Resume != nil {
						select {
						case <-runCtx.Context.Done():
							return lastEvent
						case <-runCtx.Resume:
						}
					}
				}
			}
		case err, ok := <-errCh:
			if ok {
				fmt.Println("received error:", err)
			}
			break loop
		}
	}

	return lastEvent
}
