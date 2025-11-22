package callbacks

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/plugin"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Safe execution wrappers with panic recovery for all plugin hooks

func safeExecuteBeforeNode(ctx context.Context, p plugin.Plugin, nodeName string, view *state.ReadView) (updates state.Updates, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.BeforeNode panicked: %v", r)
			updates = nil
		}
	}()
	return p.BeforeNode(ctx, nodeName, view)
}

func safeExecuteAfterNode(ctx context.Context, p plugin.Plugin, nodeName string, view *state.ReadView, updates state.Updates) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.AfterNode panicked: %v", r)
		}
	}()
	return p.AfterNode(ctx, nodeName, view, updates)
}

func safeExecuteOnNodeError(ctx context.Context, p plugin.Plugin, nodeName string, nodeErr error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnNodeError panicked: %v", r)
		}
	}()
	return p.OnNodeError(ctx, nodeName, nodeErr)
}

func safeExecuteBeforeModel(ctx context.Context, p plugin.Plugin, req *model.Request) (resp *model.Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.BeforeModel panicked: %v", r)
			resp = nil
		}
	}()
	return p.BeforeModel(ctx, req)
}

func safeExecuteAfterModel(ctx context.Context, p plugin.Plugin, req *model.Request, resp *model.Response) (result *model.Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.AfterModel panicked: %v", r)
			result = nil
		}
	}()
	return p.AfterModel(ctx, req, resp)
}

func safeExecuteOnModelError(ctx context.Context, p plugin.Plugin, req *model.Request, modelErr error) (resp *model.Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnModelError panicked: %v", r)
			resp = nil
		}
	}()
	return p.OnModelError(ctx, req, modelErr)
}

func safeExecuteBeforeTool(ctx context.Context, p plugin.Plugin, toolName string, input any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.BeforeTool panicked: %v", r)
		}
	}()
	return p.BeforeTool(ctx, toolName, input)
}

func safeExecuteAfterTool(ctx context.Context, p plugin.Plugin, toolName string, result any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.AfterTool panicked: %v", r)
		}
	}()
	return p.AfterTool(ctx, toolName, result)
}

func safeExecuteOnToolError(ctx context.Context, p plugin.Plugin, toolName string, toolErr error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnToolError panicked: %v", r)
		}
	}()
	return p.OnToolError(ctx, toolName, toolErr)
}

func safeExecuteOnStateChange(ctx context.Context, p plugin.Plugin, nodeName string, updates state.Updates) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin.OnStateChange panicked: %v", r)
		}
	}()
	return p.OnStateChange(ctx, nodeName, updates)
}
