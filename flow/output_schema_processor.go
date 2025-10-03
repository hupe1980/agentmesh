package flow

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/tool"
)

// OutputSchemaProcessor wires structured output expectations into the model request.
// It prefers native provider support and falls back to the internal set_model_response
// tool when necessary.
type OutputSchemaProcessor struct{}

// NewOutputSchemaProcessor constructs a processor that adds structured output handling
// to outgoing model requests.
func NewOutputSchemaProcessor() *OutputSchemaProcessor { return &OutputSchemaProcessor{} }

// Name returns the processor's identifier.
func (p *OutputSchemaProcessor) Name() string { return "output_schema" }

// ProcessRequest inspects the agent's declared output schema and configures the
// model request accordingly. When the underlying model advertises structured-output
// support, the schema is attached directly. Otherwise a fallback tool is registered
// so the model can emit the structured payload via function calling.
func (p *OutputSchemaProcessor) ProcessRequest(
	ctx context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent core.FlowAgent,
) error {
	req.OutputSchema = core.None[core.OutputSchema]()

	osOpt := agent.OutputSchema()
	if !osOpt.IsSet() {
		return nil
	}

	os, ok := osOpt.Get()
	if !ok {
		return nil
	}

	if agent.Model().Capabilities().SupportsStructuredOutput {
		req.OutputSchema = osOpt
		return nil
	}

	fallback, err := tool.NewSetModelResponseTool(os)
	if err != nil {
		return fmt.Errorf("failed to create set_model_response tool: %w", err)
	}

	toolCtx := core.NewToolContext(reqCtx)
	if err := fallback.ProcessModelRequest(ctx, toolCtx, req); err != nil {
		return fmt.Errorf("failed to attach set_model_response tool: %w", err)
	}

	return nil
}
