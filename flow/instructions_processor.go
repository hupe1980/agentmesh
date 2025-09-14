package flow

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
)

// InstructionsProcessor handles system prompt and instruction processing.
type InstructionsProcessor struct{}

// NewInstructionsProcessor creates a new instructions processor.
func NewInstructionsProcessor() *InstructionsProcessor { return &InstructionsProcessor{} }

// Name returns the processor's identifier.
func (p *InstructionsProcessor) Name() string { return "instructions" }

// ProcessRequest adds system instructions to the chat request.
func (p *InstructionsProcessor) ProcessRequest(
	ctx context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent Agent,
) error {
	log := logging.FromContext(ctx)
	instructions, err := agent.ResolveInstructions(ctx, reqCtx)
	if err != nil {
		return fmt.Errorf("failed to resolve instruction: %w", err)
	}

	log.Debug("agent.instruction.resolved", "agent", agent.Name(), "length", len(instructions))

	// Apply template substitution using a merged state snapshot (persisted + delta)
	snapshot := reqCtx.StateSnapshot()
	if len(snapshot) > 0 {
		rendered, tplErr := renderTemplate(instructions, snapshot)
		if tplErr != nil {
			return fmt.Errorf("failed to render template: %w", tplErr)
		}
		req.AppendInstructions(rendered)
	} else {
		req.AppendInstructions(instructions)
	}

	return nil
}

// renderTemplate replaces template variables using Go's text/template package.
// This lives in internal to avoid committing to public API stability prematurely.
func renderTemplate(text string, state map[string]any) (string, error) {
	if !strings.Contains(text, "{{") { // fast path: no template markers
		return text, nil
	}

	// Create a new template with helper funcs
	tmpl, err := template.New("prompt").Funcs(template.FuncMap{
		"default": func(defaultVal any, val any) any {
			if val == nil || val == "" {
				return defaultVal
			}
			return val
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(string(s[0])) + strings.ToLower(s[1:])
		},
		"join": func(sep string, items []any) string {
			strItems := make([]string, len(items))
			for i, item := range items {
				strItems[i] = fmt.Sprintf("%v", item)
			}
			return strings.Join(strItems, sep)
		},
	}).Parse(text)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, state); err != nil {
		return "", err
	}

	return buf.String(), nil
}
