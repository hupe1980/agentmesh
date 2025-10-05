package testutil

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
)

// MockAgent is a lightweight, function-based test double for agents.
//
// It is designed for unit tests to avoid ad-hoc mock types. Configure behavior
// by setting the exported fields or by providing function hooks. Hierarchy is
// modeled via the read-only Parent/SubAgents methods; set SubAgentsList and
// ParentAgent directly in tests to shape the tree.
//
// Example:
//
//	m := testutil.NewMockAgent("A")
//	m.SubAgentsList = []core.Agent{testutil.NewMockAgent("child")}
//	m.ResolveInstructionsFunc = func(ctx context.Context, _ core.ReadonlyContext) (string, error) {
//	    return "you are a test agent", nil
//	}
//
// All methods have safe defaults; function hooks (when set) take precedence.
// NOTE: This lives in internal/testutil and is intended only for tests. Do not
// use from production code.
//
//nolint:revive // this is a test utility with many exported fields for convenience
type MockAgent struct {
	// Static fields (used when corresponding Func hook is nil)
	NameVal                 string
	DescriptionVal          string
	ModelVal                core.Model
	ToolsList               []core.Tool
	ToolsetList             []core.Toolset
	ParentAgent             core.Agent
	SubAgentsList           []core.Agent
	StreamingEnabled        bool
	TransferToPeersEnabled  bool
	TransferToParentEnabled bool
	OutputSchemaVal         core.Opt[core.OutputSchema]
	OutputKeyVal            string
	MaxHistoryVal           int
	HistoryModeVal          core.HistoryMode
	InstructionsText        string

	// Optional function hooks to override behaviors
	// RunFunc, when set, will be invoked by Run; return its error.
	RunFunc                 func(ctx context.Context, reqCtx core.RequestContext, writer core.EventWriter) error
	RunCount                int
	ResolveInstructionsFunc func(ctx context.Context, roCtx core.ReadonlyContext) (string, error)
	TransferToAgentFunc     func(
		ctx context.Context,
		reqCtx core.RequestContext,
		queue core.EventWriter,
		agentName string,
	) error
	HasSubAgentsFunc func() bool
	SubAgentsFunc    func() []core.Agent
}

// NewMockAgent constructs a MockAgent with the given name and sensible defaults.
func NewMockAgent(name string) *MockAgent {
	return &MockAgent{
		NameVal:                 name,
		ToolsList:               []core.Tool{},
		ToolsetList:             []core.Toolset{},
		SubAgentsList:           []core.Agent{},
		StreamingEnabled:        false,
		TransferToPeersEnabled:  false,
		TransferToParentEnabled: false,
		OutputKeyVal:            "",
		MaxHistoryVal:           10,
		HistoryModeVal:          core.HistoryAll,
		InstructionsText:        "",
	}
}

// Name returns the mock's name.
func (m *MockAgent) Name() string { return m.NameVal }

// Description returns the mock's description.
func (m *MockAgent) Description() string { return m.DescriptionVal }

// Run is a no-op implementation suitable for tests that don't need execution.
func (m *MockAgent) Run(ctx context.Context, reqCtx core.RequestContext, writer core.EventWriter) error {
	m.RunCount++
	if m.RunFunc != nil {
		return m.RunFunc(ctx, reqCtx, writer)
	}
	// Default no-op
	return nil
}

// Model returns the configured model (if any).
func (m *MockAgent) Model() core.Model { return m.ModelVal }

// ModelCapabilities returns the capabilities of the underlying model.
func (m *MockAgent) ModelCapabilities() *core.ModelCapabilities {
	if m.ModelVal != nil {
		return m.ModelVal.Capabilities()
	}

	return &core.ModelCapabilities{}
}

// ResolveInstructions returns instructions using the hook when provided; otherwise InstructionsText.
func (m *MockAgent) ResolveInstructions(ctx context.Context, roCtx core.ReadonlyContext) (string, error) {
	if m.ResolveInstructionsFunc != nil {
		return m.ResolveInstructionsFunc(ctx, roCtx)
	}
	return m.InstructionsText, nil
}

// Tools returns the configured tool map.
func (m *MockAgent) ResolveTools(ctx context.Context, roCtx core.ReadonlyContext) ([]core.Tool, error) {
	// Start with locally registered tools
	allTools := append([]core.Tool(nil), m.ToolsList...)

	for _, ts := range m.ToolsetList {
		tools, err := ts.ListTools(ctx, roCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to list tools from toolset: %w", err)
		}
		allTools = append(allTools, tools...)
	}

	return allTools, nil
}

// Parent returns the configured parent agent.
func (m *MockAgent) Parent() core.Agent { return m.ParentAgent }

// RootAgent returns the top-most ancestor in the hierarchy.
func (m *MockAgent) RootAgent() core.Agent {
	if m.ParentAgent == nil {
		return m
	}

	// Traverse up to find the root
	current := m.ParentAgent
	for current.Parent() != nil {
		current = current.Parent()
	}

	return current
}

// HasSubAgents reports whether SubAgentsList is non-empty unless overridden by HasSubAgentsFunc.
func (m *MockAgent) HasSubAgents() bool {
	if m.HasSubAgentsFunc != nil {
		return m.HasSubAgentsFunc()
	}
	return len(m.SubAgentsList) > 0
}

// SubAgents returns the configured SubAgentsList unless overridden by SubAgentsFunc.
func (m *MockAgent) SubAgents() []core.Agent {
	if m.SubAgentsFunc != nil {
		return m.SubAgentsFunc()
	}
	return m.SubAgentsList
}

// SetParent assigns the parent of the mock; allows single assignment (basic mimic of real invariant).
func (m *MockAgent) SetParent(p core.Agent) error {
	if p == nil {
		// allow detach in tests
		m.ParentAgent = nil
		return nil
	}
	if m.ParentAgent != nil && m.ParentAgent != p {
		return fmt.Errorf("mockagent: parent already set")
	}
	m.ParentAgent = p
	return nil
}

// AddSubAgents appends children and sets their parent when they expose SetParent.
func (m *MockAgent) AddSubAgents(children ...core.Agent) error {
	for _, c := range children {
		if c == nil {
			continue
		}
		if setter, ok := c.(interface{ SetParent(core.Agent) error }); ok {
			if err := setter.SetParent(m); err != nil {
				return err
			}
		}
		m.SubAgentsList = append(m.SubAgentsList, c)
	}
	return nil
}

// FindAgent performs a depth-first search starting at this mock and including its SubAgents.
func (m *MockAgent) FindAgent(name string) (core.Agent, error) {
	if m.NameVal == name {
		return m, nil
	}

	return m.findSubAgent(name)
}

func (m *MockAgent) findSubAgent(name string) (core.Agent, error) {
	// Search through all child agents
	for _, sub := range m.SubAgentsList {
		if result, err := sub.FindAgent(name); err == nil {
			return result, nil
		}
	}
	return nil, core.ErrAgentNotFound
}

// IsStreamingEnabled returns the configured flag.
func (m *MockAgent) IsStreamingEnabled() bool { return m.StreamingEnabled }

// IsTransferToPeersEnabled returns the configured flag.
func (m *MockAgent) IsTransferToPeersEnabled() bool { return m.TransferToPeersEnabled }

// IsTransferToParentEnabled returns the configured flag.
func (m *MockAgent) IsTransferToParentEnabled() bool { return m.TransferToParentEnabled }

// OutputSchema returns the expected output schema for responses.
func (m *MockAgent) OutputSchema() core.Opt[core.OutputSchema] {
	return m.OutputSchemaVal
}

// OutputKey returns the configured output key.
func (m *MockAgent) OutputKey() string { return m.OutputKeyVal }

// MaxHistoryMessages returns the configured max history size.
func (m *MockAgent) MaxHistoryMessages() int { return m.MaxHistoryVal }

// HistoryMode returns what kind of history the agent receives.
func (m *MockAgent) HistoryMode() core.HistoryMode { return m.HistoryModeVal }

type MockAgentIdentity struct {
	name        string
	description string
}

func NewMockAgentIdentity(name, description string) *MockAgentIdentity {
	return &MockAgentIdentity{name: name, description: description}
}

func (a *MockAgentIdentity) Name() string        { return a.name }
func (a *MockAgentIdentity) Description() string { return a.description }

// Compile-time assertion
var _ core.AgentIdentity = (*MockAgentIdentity)(nil)
