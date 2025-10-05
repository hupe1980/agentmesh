package tool

import (
	"context"
	"encoding/json"
	"fmt"

	meshapp "github.com/hupe1980/agentmesh/app"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/runner"
)

// AgentTool is a tool that runs an agent.
type AgentTool struct {
	agent core.Agent
}

// NewAgentTool creates a new agent tool.
func NewAgentTool(agent core.Agent) *AgentTool {
	return &AgentTool{
		agent: agent,
	}
}

// Name returns the tool's identifier.
func (t *AgentTool) Name() string { return t.agent.Name() }

// Description returns the tool's description.
func (t *AgentTool) Description() string { return t.agent.Description() }

// Parameters returns the tool's parameters schema.
func (t *AgentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"__arg1": map[string]string{"title": "__arg1", "type": "string"},
		},
		"required": []string{"__arg1"},
	}
}

// ProcessModelRequest adds this tool to the provided request.
func (t *AgentTool) ProcessModelRequest(
	ctx context.Context,
	toolCtx core.ToolContext,
	req *core.ModelRequest,
) error {
	req.AddTool(t)
	return nil
}

// Call executes the agent with the provided input and returns the result.
func (t *AgentTool) Call(
	ctx context.Context,
	toolCtx core.ToolContext,
	args string,
) (any, error) {
	log := logging.FromContext(ctx)

	var argsMap map[string]any
	if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
		return nil, NewError(t.Name(), fmt.Sprintf("invalid JSON arguments: %v", err), "VALIDATION_ERROR")
	}

	toolInput, ok := argsMap["__arg1"].(string)
	if !ok {
		return nil, NewError(t.Name(), "missing required field '__arg1'", "VALIDATION_ERROR")
	}

	application := meshapp.New(t.Name(), t.agent, func(o *meshapp.Options) {
		o.Plugins = toolCtx.PluginManager().Plugins()
	})

	r := runner.New(application, func(o *runner.Options) {
		// Use a distinct key to avoid colliding with outer run_id.
		o.RunIDKey = "tool_run_id"
		o.Logger = log
		o.ArtifactStore = &artifactStoreAdapter{toolCtx: toolCtx}
	})
	defer func() {
		_ = r.Close()
	}()

	userParts := []core.Part{core.NewPartFromText(toolInput)}

	_, results, err := r.Run(ctx, toolCtx.UserID(), "tmpSession", userParts, func(o *core.RunOptions) {
		o.StateDelta = toolCtx.StateSnapshot()
	})
	if err != nil {
		return nil, NewError(t.Name(), "agent run failed: "+err.Error(), "EXECUTION_ERROR")
	}

	lastContent := ""
	for res := range results {
		if res.Event != nil {
			if delta, ok := res.Event.Actions.StateDelta.Get(); ok && delta != nil {
				toolCtx.State().Update(delta)
			}

			if res.Event.IsFinalResponse() {
				lastContent = res.Event.Text()
			}
		}
	}

	return lastContent, nil
}

type artifactStoreAdapter struct {
	toolCtx core.ToolContext
}

// Save stores (or overwrites) the artifact for the given composite key.
func (a *artifactStoreAdapter) Save(
	ctx context.Context,
	appName, userID, sessionID, fileName string,
	artifact core.Part,
) error {
	return a.toolCtx.SaveArtifact(ctx, fileName, artifact)
}

// Load returns a copy of the stored artifact or ErrNotFound.
func (a *artifactStoreAdapter) Load(
	ctx context.Context,
	appName, userID, sessionID, fileName string,
) (core.Part, error) {
	return a.toolCtx.LoadArtifact(ctx, fileName)
}

// Delete removes the artifact for the given composite key.
func (a *artifactStoreAdapter) Delete(ctx context.Context, appName, userID, sessionID, fileName string) error {
	return a.toolCtx.DeleteArtifact(ctx, fileName)
}

// ListKeys returns all artifact keys for the given session.
func (a *artifactStoreAdapter) ListKeys(ctx context.Context, appName, userID, sessionID string) ([]string, error) {
	return a.toolCtx.ListArtifactKeys(ctx)
}

// Close releases any resources held by the store.
func (a *artifactStoreAdapter) Close() error {
	return nil
}

// Compile-time assertion
var _ core.Tool = (*AgentTool)(nil)
