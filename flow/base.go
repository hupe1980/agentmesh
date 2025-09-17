package flow

import (
	"context"
	"errors"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/flow/sm"
	"github.com/hupe1980/agentmesh/logging"
)

// Executors bundles the executors required by flows.
type Executors struct {
	AgentExecutor core.AgentExecutor
	ModelExecutor core.ModelExecutor
	ToolExecutor  core.ToolExecutor
}

// BaseFlow is a minimal single‑agent flow implementation that supports a
// request → LLM → (optional tool loop) cycle with pluggable request/response processors.
type BaseFlow struct {
	agent              core.FlowAgent
	requestProcessors  []RequestProcessor
	responseProcessors []ResponseProcessor
	executors          *Executors
}

// NewBaseFlow creates a new basic single-agent flow.
func NewBaseFlow(agent core.FlowAgent, executors *Executors) *BaseFlow {
	return &BaseFlow{
		agent:              agent,
		requestProcessors:  []RequestProcessor{},
		responseProcessors: []ResponseProcessor{},
		executors:          executors,
	}
}

// AddRequestProcessor appends a request processor; order of registration defines execution order.
func (f *BaseFlow) AddRequestProcessor(processor RequestProcessor) {
	f.requestProcessors = append(f.requestProcessors, processor)
}

// AddResponseProcessor appends a response processor executed after each model chunk.
func (f *BaseFlow) AddResponseProcessor(processor ResponseProcessor) {
	f.responseProcessors = append(f.responseProcessors, processor)
}

// Execute runs the flow synchronously using a simple step-based state machine.
func (f *BaseFlow) Execute(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error {
	frame := &flowFrame{}

	m := sm.New[flowState, flowFrame](stateBuild)
	// build -> call
	m.AddTransition(stateBuild, stateCall, nil)
	// call branching
	m.AddTransition(stateCall, stateHandle, func(fr *flowFrame) bool { return len(fr.fnCalls) > 0 })
	m.AddTransition(stateCall, stateEnd, func(fr *flowFrame) bool { return len(fr.fnCalls) == 0 })
	// handle outcomes
	m.AddTransition(stateHandle, stateEnd, shouldEnd)
	m.AddTransition(stateHandle, stateBuild, shouldLoop)

	if err := m.Run(frame, func(s flowState, fr *flowFrame) error {
		switch s {
		case stateBuild:
			return f.stepBuild(ctx, reqCtx, fr)
		case stateCall:
			return f.stepCall(ctx, reqCtx, queue, fr)
		case stateHandle:
			return f.stepHandle(ctx, reqCtx, queue, fr)
		case stateEnd:
			return nil
		}
		return nil
	}); err != nil {
		return fmt.Errorf("flow execution failed: %w", err)
	}
	return nil
}

// flowState enumerates the BaseFlow state machine states.
type flowState string

const (
	stateBuild  flowState = "build"
	stateCall   flowState = "call"
	stateHandle flowState = "handle"
	stateEnd    flowState = "end"
)

// shouldEnd returns true if the flow should terminate after handle.
func shouldEnd(fr *flowFrame) bool {
	if fr.lastEvent == nil {
		return false
	}
	acts := &fr.lastEvent.Actions
	return acts.TransferToAgent.IsSet() || acts.Escalate.Or(false)
}

// shouldLoop returns true if the flow should continue another turn.
func shouldLoop(fr *flowFrame) bool {
	if fr.lastEvent == nil { // nothing happened yet → build again
		return true
	}
	acts := &fr.lastEvent.Actions
	return !acts.TransferToAgent.IsSet() && !acts.Escalate.Or(false)
}

// flowFrame carries data across steps.
type flowFrame struct {
	req       *core.ModelRequest
	lastEvent *core.Event
	fnCalls   []*core.FunctionCall
}

// handleFunctionCalls executes all function calls, merges their responses into a single
// tool event (aggregating actions), and emits it. Returns the last event pointer updated.
func (f *BaseFlow) handleFunctionCalls(
	ctx context.Context,
	reqCtx core.RequestContext,
	writer core.EventWriter,
	fnCalls []*core.FunctionCall,
	toolRegistry map[string]core.Tool,
	last *core.Event,
) (*core.Event, error) {
	log := logging.FromContext(ctx)

	// Execute all function calls (out-of-order completion)
	collected, execErr := f.executors.ToolExecutor.Execute(ctx, reqCtx, toolRegistry, fnCalls)
	if execErr != nil {
		return nil, fmt.Errorf("failed to execute function calls: %w", execErr)
	}

	if len(collected) == 0 {
		return last, nil
	}

	// Single tool call: emit as-is
	if len(collected) == 1 {
		outEv := collected[0]
		last = outEv
		if err := writer.Write(ctx, outEv); err != nil {
			log.Error("event.write.error", "error", err)
		}

		return last, nil
	}

	// Multiple function calls: merge responses deterministically by original call order
	respByID, actionsByID := indexByCallID(collected)

	tmplResp := chooseTemplateResponse(fnCalls, respByID)
	if tmplResp == nil {
		// Fallback to previous behavior if no ids matched (shouldn't happen)
		tmplResp = collected[0].GetFunctionResponses()[0]
	}

	// Build parts and merge actions in call order
	parts := buildPartsInOrder(fnCalls, respByID)
	stateDelta, artifactDelta, transferTo, escalate, skip := mergeActionsInOrder(fnCalls, actionsByID)
	merged := assembleMergedFunctionResponseEvent(
		reqCtx.RunID(), reqCtx.AgentName(), tmplResp, parts, stateDelta, artifactDelta, transferTo, escalate, skip,
	)

	last = merged

	if err := writer.Write(ctx, merged); err != nil {
		log.Error("event.write.error", "error", err)
		return nil, err
	}

	return last, nil
}

// stepBuild runs request processors and prepares the model request + tools.
func (f *BaseFlow) stepBuild(ctx context.Context, reqCtx core.RequestContext, fr *flowFrame) error {
	// Build request using processors and attach tools in deterministic order.
	req := new(core.ModelRequest)

	for _, processor := range f.requestProcessors {
		if err := processor.ProcessRequest(ctx, reqCtx, req, f.agent); err != nil {
			return fmt.Errorf("request processor %s failed: %w", processor.Name(), err)
		}
	}

	tools := f.agent.Tools()
	for _, tool := range tools {
		toolCtx := core.NewToolContext(reqCtx)

		if err := tool.ProcessModelRequest(ctx, toolCtx, req); err != nil {
			return fmt.Errorf("failed to process model request for tool %s: %w", tool.Name(), err)
		}
	}

	fr.req = req

	return nil
}

// stepCall calls the model, emits assistant events, and collects tool calls.
func (f *BaseFlow) stepCall(
	ctx context.Context,
	reqCtx core.RequestContext,
	queue core.EventWriter,
	fr *flowFrame,
) error {
	if err := reqCtx.IncrementModelCalls(); err != nil {
		return fmt.Errorf("failed to increment model calls: %w", err)
	}

	mdl := f.agent.Model()
	respCh, errCh := f.executors.ModelExecutor.Execute(ctx, reqCtx, mdl, fr.req)

	for respCh != nil || errCh != nil {
		select {
		case r, ok := <-respCh:
			if !ok {
				respCh = nil
				continue
			}
			if r == nil {
				continue
			}

			// Apply response processors to each chunk (partial or final).
			for _, processor := range f.responseProcessors {
				if err := processor.ProcessResponse(ctx, reqCtx, r, f.agent); err != nil {
					return fmt.Errorf("response processor %s failed: %w", processor.Name(), err)
				}
			}

			// Convert model response into appropriate event type.
			var ev *core.Event
			if r.Partial {
				ev = core.NewPartialAssistantEvent(reqCtx.RunID(), f.agent.Name(), r.Parts...)
			} else {
				ev = core.NewFullAssistantEvent(reqCtx.RunID(), f.agent.Name(), r.Parts...)
			}

			if err := queue.Write(ctx, ev); err != nil {
				return fmt.Errorf("failed to write event: %w", err)
			}

			fr.lastEvent = ev
			if !r.Partial {
				fr.fnCalls = ev.GetFunctionCalls()
			}
		case err, ok := <-errCh:
			if ok {
				return fmt.Errorf("model execution failed: %w", err)
			}

			errCh = nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// stepHandle executes function calls, emits tool events, and handles transfer/escalation.
func (f *BaseFlow) stepHandle(
	ctx context.Context,
	reqCtx core.RequestContext,
	writer core.EventWriter,
	fr *flowFrame,
) error {
	log := logging.FromContext(ctx)

	last, err := f.handleFunctionCalls(ctx, reqCtx, writer, fr.fnCalls, fr.req.ToolRegistry, fr.lastEvent)
	if err != nil {
		return err
	}

	fr.lastEvent = last
	if last == nil {
		return nil
	}

	if targetName := last.Actions.TransferToAgent.Or(""); targetName != "" {
		// Find the target agent
		rootAgent := f.agent.RootAgent()

		targetAgent, err := rootAgent.FindAgent(targetName)
		if err != nil || targetAgent == nil {
			if errors.Is(err, core.ErrAgentNotFound) {
				return fmt.Errorf("agent '%s' not found in hierarchy: %w", targetName, err)
			}
			return fmt.Errorf("failed to find agent '%s': %w", targetName, err)
		}

		// Execute the target agent
		if err := f.executors.AgentExecutor.Execute(ctx, reqCtx, targetAgent, writer); err != nil {
			return fmt.Errorf("failed to run agent '%s': %w", targetName, err)
		}

		log.Debug("agent.transfer.complete", "from", f.agent.Name(), "to", targetName)
	}

	return nil
}
