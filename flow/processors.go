package flow

import (
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	internalutil "github.com/hupe1980/agentmesh/internal/util"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/tool"
)

// InstructionsProcessor handles system prompt and instruction processing.
type InstructionsProcessor struct{}

// NewInstructionsProcessor creates a new instructions processor.
func NewInstructionsProcessor() *InstructionsProcessor { return &InstructionsProcessor{} }

// Name returns the processor's identifier.
func (p *InstructionsProcessor) Name() string { return "instructions" }

// ProcessRequest adds system instructions to the chat request.
func (p *InstructionsProcessor) ProcessRequest(runCtx *core.RunContext, req *model.Request, agent FlowAgent) error {
	instructions, err := agent.ResolveInstructions(runCtx)
	if err != nil {
		return fmt.Errorf("failed to resolve instruction: %w", err)
	}

	runCtx.LogDebug("agent.instruction.resolved", "agent", agent.GetName(), "length", len(instructions))

	if runCtx.Session != nil && runCtx.Session.State != nil {
		var tplErr error

		// Apply template substitution to system prompt using session state
		req.Instructions, tplErr = internalutil.RenderTemplate(instructions, runCtx.Session.State)
		if tplErr != nil {
			return fmt.Errorf("failed to render template: %w", tplErr)
		}
	} else {
		req.Instructions = instructions
	}

	return nil
}

// ContentsProcessor handles user content processing.
type ContentsProcessor struct{}

// NewContentsProcessor creates a new contents processor.
func NewContentsProcessor() *ContentsProcessor { return &ContentsProcessor{} }

// Name returns the processor's identifier.
func (p *ContentsProcessor) Name() string { return "contents" }

// ProcessRequest adds user content to the chat request.
func (p *ContentsProcessor) ProcessRequest(runCtx *core.RunContext, req *model.Request, agent FlowAgent) error {
	contents := []core.Content{{
		Role:  "system",
		Parts: []core.Part{core.TextPart{Text: req.Instructions}},
	}}

	// Add conversation history if available
	if runCtx.Session != nil {
		events := runCtx.Session.GetConversationHistory()
		if len(events) > agent.MaxHistoryMessages() {
			events = events[len(events)-agent.MaxHistoryMessages():]
		}

		for _, ev := range events {
			if ev.Content != nil && len(ev.Content.Parts) > 0 {
				contents = append(contents, *ev.Content)
			}
		}
	}

	req.Contents = contents

	return nil
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
func (p *TransferToolInjector) ProcessRequest(runCtx *core.RunContext, req *model.Request, agent FlowAgent) error {
	// Check if transfer is enabled
	if !agent.IsTransferEnabled() {
		return nil
	}
	// Only inject if the agent has sub-agents
	if len(agent.GetSubAgents()) == 0 {
		return nil
	}

	// Avoid duplicate injection
	for _, td := range req.Tools {
		if td.Function.Name == "transfer_to_agent" {
			return nil
		}
	}

	// Create transfer tool definition
	tool := tool.NewTransferToAgentTool()

	req.AddTool(tool)

	runCtx.LogDebug("agent.transfer.tool.injected", "agent", agent.GetName())

	return nil
}
