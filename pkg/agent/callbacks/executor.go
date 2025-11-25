package callbacks

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/internal/safego"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/plugin"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Safe execution wrappers with panic recovery for all plugin hooks

func safeExecuteBeforeNode(ctx context.Context, p plugin.Plugin, nodeName string, view state.ReadView) (*graph.Command, error) {
	return safego.CallWith(
		func() (*graph.Command, error) {
			return p.BeforeNode(ctx, nodeName, view)
		},
		func(r any) error {
			return fmt.Errorf("plugin.BeforeNode panicked: %v", r)
		},
	)
}

func safeExecuteAfterNode(ctx context.Context, p plugin.Plugin, nodeName string, view state.ReadView, updates state.Updates) error {
	return safego.RunWith(
		func() error {
			return p.AfterNode(ctx, nodeName, view, updates)
		},
		func(r any) error {
			return fmt.Errorf("plugin.AfterNode panicked: %v", r)
		},
	)
}

func safeExecuteOnNodeError(ctx context.Context, p plugin.Plugin, nodeName string, nodeErr error) error {
	return safego.RunWith(
		func() error {
			return p.OnNodeError(ctx, nodeName, nodeErr)
		},
		func(r any) error {
			return fmt.Errorf("plugin.OnNodeError panicked: %v", r)
		},
	)
}

func safeExecuteBeforeModel(ctx context.Context, p plugin.Plugin, req *model.Request) (*model.Response, error) {
	return safego.CallWith(
		func() (*model.Response, error) {
			return p.BeforeModel(ctx, req)
		},
		func(r any) error {
			return fmt.Errorf("plugin.BeforeModel panicked: %v", r)
		},
	)
}

func safeExecuteAfterModel(ctx context.Context, p plugin.Plugin, req *model.Request, resp *model.Response) (*model.Response, error) {
	return safego.CallWith(
		func() (*model.Response, error) {
			return p.AfterModel(ctx, req, resp)
		},
		func(r any) error {
			return fmt.Errorf("plugin.AfterModel panicked: %v", r)
		},
	)
}

func safeExecuteOnModelError(ctx context.Context, p plugin.Plugin, req *model.Request, modelErr error) (*model.Response, error) {
	return safego.CallWith(
		func() (*model.Response, error) {
			return p.OnModelError(ctx, req, modelErr)
		},
		func(r any) error {
			return fmt.Errorf("plugin.OnModelError panicked: %v", r)
		},
	)
}

func safeExecuteBeforeTool(ctx context.Context, p plugin.Plugin, toolName string, input any) error {
	return safego.RunWith(
		func() error {
			return p.BeforeTool(ctx, toolName, input)
		},
		func(r any) error {
			return fmt.Errorf("plugin.BeforeTool panicked: %v", r)
		},
	)
}

func safeExecuteAfterTool(ctx context.Context, p plugin.Plugin, toolName string, result any) error {
	return safego.RunWith(
		func() error {
			return p.AfterTool(ctx, toolName, result)
		},
		func(r any) error {
			return fmt.Errorf("plugin.AfterTool panicked: %v", r)
		},
	)
}

func safeExecuteOnToolError(ctx context.Context, p plugin.Plugin, toolName string, toolErr error) error {
	return safego.RunWith(
		func() error {
			return p.OnToolError(ctx, toolName, toolErr)
		},
		func(r any) error {
			return fmt.Errorf("plugin.OnToolError panicked: %v", r)
		},
	)
}

func safeExecuteOnStateChange(ctx context.Context, p plugin.Plugin, nodeName string, updates state.Updates) error {
	return safego.RunWith(
		func() error {
			return p.OnStateChange(ctx, nodeName, updates)
		},
		func(r any) error {
			return fmt.Errorf("plugin.OnStateChange panicked: %v", r)
		},
	)
}
