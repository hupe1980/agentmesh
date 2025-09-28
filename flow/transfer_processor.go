package flow

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/tool"
)

// TransferToolInjector injects the transfer_to_agent tool definition when transfer is enabled.
type TransferToolInjector struct{}

// NewTransferToolInjector creates a processor that injects the transfer_to_agent tool definition.
func NewTransferToolInjector() *TransferToolInjector { return &TransferToolInjector{} }

// Name returns the unique identifier for the transfer tool injector processor.
func (p *TransferToolInjector) Name() string { return "transfer_tool_injector" }

// ProcessRequest conditionally appends the transfer_to_agent tool definition.
func (p *TransferToolInjector) ProcessRequest(
	ctx context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent core.FlowAgent,
) error {
	log := logging.FromContext(ctx)

	targets := getTransferTargets(agent)
	if len(targets) == 0 {
		log.Debug("agent.transfer.tool.skip", "agent", agent.Name(), "reason", "no targets")
		return nil
	}

	t, err := tool.NewTransferToAgentTool()
	if err != nil {
		return fmt.Errorf("failed to create transfer tool: %w", err)
	}

	toolCtx := core.NewToolContext(reqCtx)
	if err := t.ProcessModelRequest(ctx, toolCtx, req); err != nil {
		return fmt.Errorf("failed to process model request for transfer tool: %w", err)
	}

	if si := buildTargetAgentsInstructions(agent, targets); si != "" {
		req.AppendInstructions(si)
	}

	log.Debug("agent.transfer.tool.injected", "agent", agent.Name())
	return nil
}

// getTransferTargets returns allowable transfer targets.
func getTransferTargets(agent core.FlowAgent) []core.Agent {
	targets := append([]core.Agent{}, agent.SubAgents()...)
	parent := agent.Parent()
	if parent == nil {
		return targets
	}

	if _, ok := parent.(core.FlowAgent); !ok { // parent must satisfy orchestration view
		return targets
	}

	if agent.IsTransferToParentEnabled() {
		targets = append(targets, parent)
	}

	if agent.IsTransferToPeersEnabled() {
		for _, peer := range parent.SubAgents() {
			if peer.Name() != agent.Name() {
				targets = append(targets, peer)
			}
		}
	}
	return targets
}

// buildTargetAgentsInstructions builds guidance for transfer choices.
func buildTargetAgentsInstructions(agent core.FlowAgent, targets []core.Agent) string {
	buildInfo := func(a core.Agent) string {
		return fmt.Sprintf("Agent name: %s\nAgent description: %s\n", a.Name(), a.Description())
	}
	infos := make([]string, 0, len(targets))
	for _, ta := range targets {
		infos = append(infos, buildInfo(ta))
	}

	guidance := fmt.Sprintf(
		"You have a list of other agents to transfer to:\n\n%s\n"+
			"If you are the best to answer the question according to your description, answer it directly.\n\n"+
			"If another agent is better for answering the question according to its description, call `transfer_to_agent` "+
			"function to transfer the question to that agent. When transferring, do not generate any text other than "+
			"the function call",
		strings.Join(infos, "\n"),
	)

	if agent.IsTransferToParentEnabled() && agent.Parent() != nil {
		guidance += fmt.Sprintf(
			"\n\nYour parent agent is %s. "+
				"If neither the other agents nor you are best for answering the question according to the descriptions, "+
				"transfer to your parent agent.",
			agent.Parent().Name(),
		)
	}

	return guidance
}
