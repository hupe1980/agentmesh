package flow

import (
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/tool"
)

// BaseFlow is a minimal single‑agent flow implementation that supports a
// request -> LLM -> (optional tool loop) cycle with pluggable pre/post processors.
type BaseFlow struct {
	agent              FlowAgent
	requestProcessors  []RequestProcessor
	responseProcessors []ResponseProcessor
	functionExecutor   FunctionExecutor
}

// NewBaseFlow creates a new basic single-agent flow.
func NewBaseFlow(agent FlowAgent) *BaseFlow {
	return &BaseFlow{
		agent:              agent,
		requestProcessors:  []RequestProcessor{},
		responseProcessors: []ResponseProcessor{},
		functionExecutor:   NewParallelFunctionExecutor(FunctionExecutorConfig{MaxParallel: 4, PreserveOrder: true}),
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
func (f *BaseFlow) Execute(runCtx *core.RunContext) (<-chan core.Event, <-chan error, error) {
	eventChan := make(chan core.Event, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(eventChan)
		defer close(errChan)

		for {
			last, err := f.runOnce(runCtx, eventChan)
			if err != nil {
				// Propagate terminal error then stop.
				errChan <- fmt.Errorf("flow execution failed: %w", err)
				return
			}
			if last == nil { // termination (model stream closed with no event)
				return
			}
			// If we just emitted a function response without transfer/escalation, allow another LLM turn.
			if len(last.GetFunctionResponses()) > 0 {
				if last.Actions.TransferToAgent == nil && last.Actions.Escalate == nil {
					continue
				}
				// transfer or escalation ends the originating agent's loop
				return
			}
			if last.IsPartial() { // unexpected partial as last event
				return
			}
			if last.IsFinalResponse() {
				return
			}
		}
	}()

	return eventChan, errChan, nil
}

// runOnce performs one model turn (including any tool executions) and returns
// the last emitted Event (final or intermediate). A nil return signals termination.
func (f *BaseFlow) runOnce(runCtx *core.RunContext, eventChan chan<- core.Event) (*core.Event, error) {
	// Refresh session snapshot so request processors see latest conversation (including tool responses)
	if runCtx.SessionStore != nil {
		if latest, err := runCtx.SessionStore.Get(runCtx.SessionID); err == nil && latest != nil {
			runCtx.Session = latest
		}
	}

	// Create a new model request
	req := new(model.Request)

	// Run request processors
	for _, processor := range f.requestProcessors {
		if err := processor.ProcessRequest(runCtx, req, f.agent); err != nil {
			return nil, fmt.Errorf("request processor %s failed: %w", processor.Name(), err)
		}
	}

	// Build tool definitions and raw tool registry
	for _, t := range f.agent.GetTools() {
		req.AddTool(t)
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
					return nil, fmt.Errorf("response processor %s failed: %w", processor.Name(), err)
				}
			}

			ev := core.NewAssistantEvent(runCtx.RunID, f.agent.GetName(), resp.Content, resp.Partial)
			lastEvent = &ev

			// Emit processed event
			eventChan <- ev

			// Wait for session persistence (runner sends resume after append)
			if !ev.IsPartial() && runCtx.Resume != nil {
				select {
				case <-runCtx.Context.Done():
					return lastEvent, nil
				case <-runCtx.Resume:
				}
			}

			// Handle function calls
			if fnCalls := ev.GetFunctionCalls(); len(fnCalls) > 0 {
				lastEvent = f.handleFunctionCalls(runCtx, eventChan, fnCalls, req.RawTools, lastEvent)
				// If transfer requested, execute sub-agent inline (same event stream) and append its final event
				if lastEvent != nil && lastEvent.Actions.TransferToAgent != nil {
					targetName := *lastEvent.Actions.TransferToAgent
					if ma, ok := f.agent.(interface {
						TransferToAgent(*core.RunContext, string) error
					}); ok {
						// Reuse run context emission channel; create child context for sub-agent
						childCtx := runCtx.Clone()
						childCtx.Agent = core.AgentInfo{Name: targetName, Type: "unknown"}
						// Run sub-agent; its events will flow through parent agent's Run since runner only sees the aggregate channel
						if err := ma.TransferToAgent(childCtx, targetName); err != nil {
							runCtx.LogError("agent.transfer.execute.error", "from", f.agent.GetName(), "to", targetName, "error", err.Error())
						} else {
							runCtx.LogDebug("agent.transfer.complete", "from", f.agent.GetName(), "to", targetName)
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

	return lastEvent, nil
}

// handleFunctionCalls executes all function calls, merges their responses into a single
// tool event (aggregating actions) and emits it. Returns the last event pointer updated.
func (f *BaseFlow) handleFunctionCalls(
	runCtx *core.RunContext,
	eventChan chan<- core.Event,
	fnCalls []core.FunctionCall,
	toolRegistry map[string]tool.Tool,
	last *core.Event,
) *core.Event {
	collected := make([]core.Event, 0, len(fnCalls))
	collect := func(outEv core.Event) error { collected = append(collected, outEv); last = &outEv; return nil }
	f.functionExecutor.Execute(runCtx, f.agent, toolRegistry, fnCalls, collect)
	if len(collected) == 0 {
		return last
	}
	if len(collected) == 1 { // single tool call
		outEv := collected[0]
		last = &outEv
		select {
		case <-runCtx.Context.Done():
			return last
		case eventChan <- outEv:
		}
		if runCtx.Resume != nil {
			_ = runCtx.WaitForResume()
		}
		return last
	}
	firstResp := collected[0].GetFunctionResponses()[0]
	merged := core.NewFunctionResponseEvent(f.agent.GetName(), firstResp.ID, firstResp.Name, firstResp.Response, nil)
	// gather parts & actions
	parts := make([]core.Part, 0, len(collected))
	stateDelta := map[string]any{}
	artifactDelta := map[string]int{}
	var transferTo *string
	var escalate *bool
	for _, ce := range collected {
		for _, fr := range ce.GetFunctionResponses() {
			parts = append(parts, core.FunctionResponsePart{FunctionResponse: fr})
		}
		for k, v := range ce.Actions.StateDelta {
			stateDelta[k] = v
		}
		for k, v := range ce.Actions.ArtifactDelta {
			artifactDelta[k] = v
		}
		if transferTo == nil && ce.Actions.TransferToAgent != nil {
			transferTo = ce.Actions.TransferToAgent
		}
		if escalate == nil && ce.Actions.Escalate != nil {
			escalate = ce.Actions.Escalate
		}
	}
	if merged.Content != nil {
		merged.Content.Parts = parts
	}
	if len(stateDelta) > 0 {
		merged.Actions.StateDelta = stateDelta
	}
	if len(artifactDelta) > 0 {
		merged.Actions.ArtifactDelta = artifactDelta
	}
	if transferTo != nil {
		merged.Actions.TransferToAgent = transferTo
	}
	if escalate != nil {
		merged.Actions.Escalate = escalate
	}
	last = &merged
	select {
	case <-runCtx.Context.Done():
		return last
	case eventChan <- merged:
	}
	if runCtx.Resume != nil {
		_ = runCtx.WaitForResume()
	}
	return last
}
