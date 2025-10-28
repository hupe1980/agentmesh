package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	am "github.com/hupe1980/agentmesh"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model/openai"
	"github.com/hupe1980/agentmesh/tool"
)

const (
	userID    = "demo-user"
	sessionID = "long-running-session"
)

type ApprovalArgs struct {
	Purpose string  `json:"purpose" jsonschema:"description=Purpose of the reimbursement request."`
	Amount  float64 `json:"amount" jsonschema:"description=Amount of the reimbursement in USD."`
}

func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	approvalTool, err := newApprovalTool()
	if err != nil {
		log.Fatalf("create long-running tool: %v", err)
	}

	model := openai.NewModel(func(o *openai.Options) {
		o.Temperature = 0
	})

	agent, err := am.NewModelAgent("ExpenseAgent", model, func(o *am.ModelAgentOptions) {
		o.Instructions = core.NewInstructionsFromText("You help process reimbursements. Always call ask_for_approval before confirming a request, and once approval is received, clearly state who approved it in your response.")
		o.Tools = []core.Tool{approvalTool}
		o.EnableStreaming = false
	})
	if err != nil {
		log.Fatalf("create agent: %v", err)
	}

	application := am.NewApp("long_running_demo", agent)

	r := am.NewRunner(application, func(o *am.RunnerOptions) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() {
		_ = r.Close()
	}()

	initialParts := []core.Part{core.NewPartFromText("I need approval to reimburse $1200 for our team offsite.")}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	callID, pending, err := runInitial(ctx, r, initialParts)
	if err != nil {
		log.Fatalf("initial run failed: %v", err)
	}

	if callID == "" || pending == nil {
		log.Fatal("expected long-running tool invocation")
	}

	followCtx, followCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer followCancel()

	if err := resumeWithApproval(followCtx, r, callID, pending); err != nil {
		log.Fatalf("follow-up run failed: %v", err)
	}

}

func runInitial(ctx context.Context, r core.Runner, parts []core.Part) (string, map[string]any, error) {
	runID, stream, err := r.Run(ctx, userID, sessionID, parts)
	if err != nil {
		return "", nil, fmt.Errorf("run agent: %w", err)
	}

	fmt.Printf("=== Long Running Tool Demo (runID=%s) ===\n", runID)

	callID, pending, err := consumeStream("initial run", stream)
	if err != nil {
		return "", nil, err
	}

	return callID, pending, nil
}

func resumeWithApproval(ctx context.Context, r core.Runner, callID string, pending map[string]any) error {
	if callID == "" || pending == nil {
		return fmt.Errorf("no approval request to resume")
	}

	ticketID, _ := pending["ticket_id"].(string)
	fmt.Printf("\nSimulating reviewer approval for ticket %s...\n", ticketID)

	updatePayload := map[string]any{
		"status":    "approved",
		"approver":  pending["approver"],
		"purpose":   pending["purpose"],
		"amount":    pending["amount"],
		"ticket_id": pending["ticket_id"],
	}

	parts := []core.Part{
		core.NewPartFromFunctionResponse(callID, "ask_for_approval", updatePayload),
	}

	_, stream, err := r.Run(ctx, userID, sessionID, parts)
	if err != nil {
		return fmt.Errorf("resume run: %w", err)
	}

	_, _, err = consumeStream("follow-up run", stream)
	return err
}

func consumeStream(label string, stream <-chan core.RunResult) (string, map[string]any, error) {
	var (
		callID  string
		pending map[string]any
	)

	for res := range stream {
		if res.Err != nil {
			return "", nil, fmt.Errorf("%s error: %w", label, res.Err)
		}

		if res.Event == nil {
			continue
		}

		if callID == "" {
			callID = extractCallID(res.Event)
		}

		if pending == nil {
			pending = extractPending(callID, res.Event)
		}

		printEvent(res.Event)
	}

	return callID, pending, nil
}

func extractCallID(ev *core.Event) string {
	if ev == nil {
		return ""
	}

	if ids, ok := ev.LongRunningToolIDs.Get(); ok && len(ids) > 0 {
		return ids[0]
	}

	return ""
}

func extractPending(callID string, ev *core.Event) map[string]any {
	if ev == nil || callID == "" {
		return nil
	}

	for _, fr := range ev.GetFunctionResponses() {
		if callID == fr.ID {
			if payload, ok := fr.Response.(map[string]any); ok {
				return payload
			}
		}
	}

	return nil
}

func newApprovalTool() (core.Tool, error) {
	lrTool, err := tool.NewLongRunningToolFromType(
		"ask_for_approval",
		"Create an approval ticket for an expense request.",
		func(ctx context.Context, tc core.ToolContext, args ApprovalArgs) (any, error) {
			ticketID, approver := createApprovalTicket(ctx, args.Purpose, args.Amount)
			fmt.Printf("   [tool] Created approval ticket %s for %s ($%.2f)\n", ticketID, args.Purpose, args.Amount)

			return map[string]any{
				"status":    "pending",
				"approver":  approver,
				"purpose":   args.Purpose,
				"amount":    args.Amount,
				"ticket_id": ticketID,
			}, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return lrTool, nil
}

func createApprovalTicket(_ context.Context, purpose string, amount float64) (string, string) {
	_ = purpose
	_ = amount
	return "approval-ticket-1", "Sean Zhou"
}

func printEvent(ev *core.Event) {
	if ev == nil || len(ev.Parts) == 0 {
		return
	}

	fmt.Printf("\n[%s]\n", ev.Author)
	for _, part := range ev.Parts {
		switch p := part.(type) {
		case *core.TextPart:
			fmt.Println(p.Text)
		case *core.FunctionCallPart:
			fmt.Printf("function_call %s %s\n", p.FunctionCall.Name, p.FunctionCall.Arguments)
		case *core.FunctionResponsePart:
			fmt.Printf("function_response %s => %v\n", p.FunctionResponse.Name, p.FunctionResponse.Response)
		}
	}

	if ids, ok := ev.LongRunningToolIDs.Get(); ok && len(ids) > 0 {
		fmt.Printf("long_running_tool_ids: %v\n", ids)
	}
}
