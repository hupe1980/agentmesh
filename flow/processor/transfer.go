package processor

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/tool"
)

// agentTransferView is the subset of a flow.Agent required for transfer tool logic.
type agentTransferView interface {
	Name() string
	SubAgents() []core.Agent
	Parent() core.Agent
	IsTransferToParentEnabled() bool
	IsTransferToPeersEnabled() bool
}

// TransferToolInjector injects the transfer_to_agent tool definition when transfer is enabled.
type TransferToolInjector struct{}

// NewTransferToolInjector creates a processor that injects the transfer_to_agent tool
// definition into a model request when transfer is enabled and sub-agents exist.
func NewTransferToolInjector() *TransferToolInjector { return &TransferToolInjector{} }

// Name returns the unique identifier for the transfer tool injector processor.
func (p *TransferToolInjector) Name() string { return "transfer_tool_injector" }

// ProcessRequest conditionally appends the transfer_to_agent tool definition to the
// outgoing model request so the LLM can choose to call it. It is idempotent and will
// not add duplicates.
func (p *TransferToolInjector) ProcessRequest(
	ctx context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent agentTransferView,
) error {
	log := logging.FromContext(ctx)

	targets := getTransferTargets(agent)
	if len(targets) == 0 {
		log.Debug("agent.transfer.tool.skip", "agent", agent.Name(), "reason", "no targets")
		return nil
	}

	t := tool.NewTransferToAgentTool()
	toolCtx := core.NewToolContext(reqCtx)

	if err := t.ProcessModelRequest(ctx, toolCtx, req); err != nil {
		return fmt.Errorf("failed to process model request for transfer tool: %w", err)
	}

	// Provide the model with guidance on available targets (sub-agents, parent, peers) and how to transfer
	if si := buildTargetAgentsInstructions(agent, targets); si != "" {
		req.AppendInstructions(si)
	}

	log.Debug("agent.transfer.tool.injected", "agent", agent.Name())

	return nil
}

// getTransferTargets returns the list of agents that the given agent is allowed
// to transfer control to, based on hierarchy and transfer settings.
func getTransferTargets(agent agentTransferView) []core.Agent {
	// Always include direct sub-agents
	targets := append([]core.Agent{}, agent.SubAgents()...)

	parent := agent.Parent()
	if parent == nil {
		return targets
	}

	// Parent must also be a FlowAgent to support transfers
	// parent must implement the same subset for transfers (duck typing)
	if _, ok := parent.(agentTransferView); !ok {
		return targets
	}

	// Optionally include parent
	if agent.IsTransferToParentEnabled() {
		targets = append(targets, parent)
	}

	// Optionally include peers (siblings)
	if agent.IsTransferToPeersEnabled() {
		for _, peer := range parent.SubAgents() {
			if peer.Name() != agent.Name() {
				targets = append(targets, peer)
			}
		}
	}

	return targets
}

// buildTargetAgentsInstructions builds a natural-language instruction block
// describing available transfer targets and when to call the transfer tool.
func buildTargetAgentsInstructions(agent agentTransferView, targets []core.Agent) string {
	// Helper to format single agent info
	buildInfo := func(a core.Agent) string {
		return fmt.Sprintf("Agent name: %s\nAgent description: %s\n", a.Name(), a.Description())
	}

	// Assemble list of target infos
	infos := make([]string, 0, len(targets))
	for _, ta := range targets {
		infos = append(infos, buildInfo(ta))
	}

	guidance := fmt.Sprintf(
		"You have a list of other agents to transfer to:\n\n%s\n"+
			"If you are the best to answer the question according to your description, answer it directly.\n\n"+
			"If another agent is better for answering the question according to its description, call `transfer_to_agent` "+
			"function to transfer the question to that agent. "+
			"When transferring, do not generate any text other than the function call",
		strings.Join(infos, "\n"),
	)

	// If parent exists and transfer-to-parent is enabled, add transfer to parent hint
	if agent.IsTransferToParentEnabled() && agent.Parent() != nil {
		guidance += fmt.Sprintf(
			"\n\nYour parent agent is %s. If neither the other agents nor you are best for answering the question "+
				"according to the descriptions, transfer to your parent agent.",
			agent.Parent().Name(),
		)
	}

	return guidance
}
