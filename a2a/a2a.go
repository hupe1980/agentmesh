package a2a

import (
	"context"

	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/hupe1980/agentmesh/core"
)

// agentExecutor is responsible for executing agent tasks.
type agentExecutor struct {
	engine core.Runner
}

// NewAgentExecutor creates a new agent executor.
func NewAgentExecutor(engine core.Runner) a2asrv.AgentExecutor {
	return &agentExecutor{engine: engine}
}

// Execute invokes an agent with the provided context and translates agent outputs
// into A2A events writing them to the provided event queue.
//
// Returns an error if agent invocation failed.
func (ae *agentExecutor) Execute(ctx context.Context, reqCtx a2asrv.RequestContext, queue a2asrv.EventWriter) error {
	//sessionID := reqCtx.ContextID
	return nil
}

// Cancel requests the agent to stop processing an ongoing task.
//
// The agent should attempt to gracefully stop the task identified by the
// task ID in the request context and publish a TaskStatusUpdateEvent with
// state TaskStateCanceled to the event queue.
//
// Returns an error if the cancellation request cannot be processed.
func (ae *agentExecutor) Cancel(ctx context.Context, reqCtx a2asrv.RequestContext, queue a2asrv.EventWriter) error {
	if err := ae.engine.Cancel(string(reqCtx.TaskID)); err != nil {
		return err
	}

	return nil
}
