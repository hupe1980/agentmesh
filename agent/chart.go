package agent

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/mermaid"
)

// FlowchartOption configures how an agent flowchart is rendered.
type FlowchartOption func(*flowchartOptions)

type flowchartOptions struct {
	direction           string
	includeDescriptions bool
	includeTools        bool
}

func defaultFlowchartOptions() flowchartOptions {
	return flowchartOptions{
		direction:           "TD",
		includeDescriptions: true,
		includeTools:        true,
	}
}

// WithDirection sets the Mermaid graph direction (e.g. TD, LR, BT).
func WithDirection(direction string) FlowchartOption {
	direction = strings.TrimSpace(direction)
	if direction == "" {
		return func(*flowchartOptions) {}
	}

	direction = strings.ToUpper(direction)

	return func(o *flowchartOptions) {
		o.direction = direction
	}
}

// WithDescriptions toggles embedding agent/tool descriptions in node labels.
func WithDescriptions(include bool) FlowchartOption {
	return func(o *flowchartOptions) {
		o.includeDescriptions = include
	}
}

// WithTools toggles rendering tool nodes for agents that expose them.
func WithTools(include bool) FlowchartOption {
	return func(o *flowchartOptions) {
		o.includeTools = include
	}
}

// Flowchart creates a Mermaid flowchart for the agent hierarchy rooted at root.
func Flowchart(root core.Agent, optFns ...FlowchartOption) (string, error) {
	if root == nil {
		return "", fmt.Errorf("agent: root agent is nil")
	}

	opts := defaultFlowchartOptions()

	for _, fn := range optFns {
		if fn != nil {
			fn(&opts)
		}
	}

	builder := newAgentFlowchartBuilder(opts)
	builder.visit(root)

	return builder.chart.Render(), nil
}

type toolProvider interface {
	Tools() []core.Tool
}

type agentFlowchartBuilder struct {
	opts      flowchartOptions
	chart     *mermaid.Flowchart
	allocator *idAllocator
	agentIDs  map[core.Agent]string
	visited   map[core.Agent]bool
}

func newAgentFlowchartBuilder(opts flowchartOptions) *agentFlowchartBuilder {
	return &agentFlowchartBuilder{
		opts:      opts,
		chart:     mermaid.NewFlowchart(mermaid.WithDirection(opts.direction)),
		allocator: newIDAllocator(),
		agentIDs:  make(map[core.Agent]string),
		visited:   make(map[core.Agent]bool),
	}
}

func (b *agentFlowchartBuilder) visit(agent core.Agent) {
	if agent == nil || b.visited[agent] {
		return
	}

	b.visited[agent] = true

	agentID := b.ensureAgent(agent)

	if b.opts.includeTools {
		b.appendTools(agent, agentID)
	}

	children := append([]core.Agent(nil), agent.SubAgents()...)
	sort.SliceStable(children, func(i, j int) bool {
		return strings.ToLower(children[i].Name()) < strings.ToLower(children[j].Name())
	})

	if len(children) > 0 {
		childIDs := make([]string, len(children))
		for i, child := range children {
			childIDs[i] = b.ensureAgent(child)
		}

		b.chart.AddEdge(agentID, childIDs[0], "")

		for i := 0; i < len(childIDs)-1; i++ {
			b.chart.AddEdge(childIDs[i], childIDs[i+1], "")
		}
	}

	for _, child := range children {
		b.visit(child)
	}
}

func (b *agentFlowchartBuilder) ensureAgent(agent core.Agent) string {
	if id, ok := b.agentIDs[agent]; ok {
		return id
	}

	label := agent.Name()
	if label == "" {
		label = "(unnamed agent)"
	}

	if b.opts.includeDescriptions {
		if desc := strings.TrimSpace(agent.Description()); desc != "" {
			label = label + "\n" + desc
		}
	}

	id := b.allocator.new(agent.Name())
	b.chart.AddNode(id, label, mermaid.ShapeDefault)
	b.agentIDs[agent] = id

	return id
}

func (b *agentFlowchartBuilder) appendTools(agent core.Agent, agentID string) {
	provider, ok := agent.(toolProvider)
	if !ok {
		return
	}

	tools := provider.Tools()
	if len(tools) == 0 {
		return
	}

	sort.SliceStable(tools, func(i, j int) bool {
		return strings.ToLower(tools[i].Name()) < strings.ToLower(tools[j].Name())
	})

	for idx, tool := range tools {
		label := "Tool: " + tool.Name()
		if b.opts.includeDescriptions {
			if desc := strings.TrimSpace(tool.Description()); desc != "" {
				label = label + "\n" + desc
			}
		}

		toolID := b.allocator.new(fmt.Sprintf("%s_tool_%d", agentID, idx))
		b.chart.AddNode(toolID, label, mermaid.ShapeSubroutine)
		b.chart.AddEdgeWithStyle(agentID, toolID, "uses", mermaid.EdgeDashed)
	}
}

type idAllocator struct {
	counts map[string]int
}

func newIDAllocator() *idAllocator {
	return &idAllocator{counts: make(map[string]int)}
}

func (a *idAllocator) new(base string) string {
	sanitized := sanitizeIdentifier(base)
	if sanitized == "" {
		sanitized = "node"
	}

	count := a.counts[sanitized]
	if count == 0 {
		a.counts[sanitized] = 1
		return sanitized
	}

	id := fmt.Sprintf("%s_%d", sanitized, count)
	a.counts[sanitized] = count + 1

	return id
}

func sanitizeIdentifier(value string) string {
	if value == "" {
		return ""
	}

	var b strings.Builder

	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}

	sanitized := b.String()
	if sanitized == "" {
		return ""
	}

	first := sanitized[0]
	if first != '_' && !unicode.IsLetter(rune(first)) {
		sanitized = "_" + sanitized
	}

	return sanitized
}
