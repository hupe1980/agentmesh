package callbacks

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// Safe execution wrappers with panic recovery for all plugin hooks

func safeExecuteOnGraphStart(ctx context.Context, p Plugin, graphID string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnGraphStart panicked: %v", r)
		}
	}()
	return p.OnGraphStart(ctx, graphID)
}

func safeExecuteOnGraphComplete(ctx context.Context, p Plugin, graphID string, stats GraphStats) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnGraphComplete panicked: %v", r)
		}
	}()
	return p.OnGraphComplete(ctx, graphID, stats)
}

func safeExecuteOnGraphError(ctx context.Context, p Plugin, graphID string, graphErr error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnGraphError panicked: %v", r)
		}
	}()
	return p.OnGraphError(ctx, graphID, graphErr)
}

func safeExecuteBeforeNode(ctx context.Context, p Plugin, nodeName string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.BeforeNode panicked: %v", r)
		}
	}()
	return p.BeforeNode(ctx, nodeName)
}

func safeExecuteAfterNode(ctx context.Context, p Plugin, nodeName string, result NodeResult) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.AfterNode panicked: %v", r)
		}
	}()
	return p.AfterNode(ctx, nodeName, result)
}

func safeExecuteBeforeModel(ctx context.Context, p Plugin, req *model.Request) (resp *model.Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.BeforeModel panicked: %v", r)
			resp = nil
		}
	}()
	return p.BeforeModel(ctx, req)
}

func safeExecuteAfterModel(ctx context.Context, p Plugin, req *model.Request, resp *model.Response) (result *model.Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.AfterModel panicked: %v", r)
			result = nil
		}
	}()
	return p.AfterModel(ctx, req, resp)
}

func safeExecuteOnModelError(ctx context.Context, p Plugin, req *model.Request, modelErr error) (resp *model.Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnModelError panicked: %v", r)
			resp = nil
		}
	}()
	return p.OnModelError(ctx, req, modelErr)
}

func safeExecuteBeforeTool(ctx context.Context, p Plugin, toolName string, input any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.BeforeTool panicked: %v", r)
		}
	}()
	return p.BeforeTool(ctx, toolName, input)
}

func safeExecuteAfterTool(ctx context.Context, p Plugin, toolName string, result ToolResult) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.AfterTool panicked: %v", r)
		}
	}()
	return p.AfterTool(ctx, toolName, result)
}

func safeExecuteOnToolError(ctx context.Context, p Plugin, toolName string, toolErr error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnToolError panicked: %v", r)
		}
	}()
	return p.OnToolError(ctx, toolName, toolErr)
}

func safeExecuteOnStateChange(ctx context.Context, p Plugin, changes StateChanges) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnStateChange panicked: %v", r)
		}
	}()
	return p.OnStateChange(ctx, changes)
}

func safeExecuteOnMessage(ctx context.Context, p Plugin, msg message.Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnMessage panicked: %v", r)
		}
	}()
	return p.OnMessage(ctx, msg)
}
