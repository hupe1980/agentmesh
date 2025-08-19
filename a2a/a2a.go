package a2a

import (
	"context"
	"fmt"

	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/hupe1980/agentmesh/core"
)

type agentExecutor struct {
	engine core.Runner
}

func NewAgentExecutor(engine core.Runner) a2asrv.AgentExecutor {
	return &agentExecutor{engine: engine}
}

func (ae *agentExecutor) Execute(ctx context.Context, reqCtx a2asrv.RequestContext, queue a2asrv.EventWriter) error {
	sessionID := reqCtx.ContextID
	msg := fmt.Sprint(reqCtx.Request.Message)
	content := core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: msg}}}
	_, eventsCh, errorsCh, err := ae.engine.Run(ctx, sessionID, content)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-eventsCh:
			if !ok {
				select {
				case err := <-errorsCh:
					return err
				default:
					return nil
				}
			}
			_ = ev // TODO: forward to queue once mapping defined
		case err := <-errorsCh:
			if err != nil {
				return err
			}
		}
	}
}

func (ae *agentExecutor) Cancel(ctx context.Context, reqCtx a2asrv.RequestContext, queue a2asrv.EventWriter) error {
	// Cancellation semantics: currently sessionID == invocationID external mapping missing.
	return ae.engine.Cancel(reqCtx.ContextID)
}
