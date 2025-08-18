package core

// Agent defines the core interface that all agents in AgentMesh must implement.
//
// Agents are the primary processing units in the AgentMesh framework. They receive
// inputs through an InvocationContext, process them asynchronously, and emit events
// to communicate results and state changes back to the Runner.
//
// The Agent interface supports both simple single-agent scenarios and complex
// hierarchical multi-agent workflows through the sub-agent management methods.
//
// Implementations must:
//   - Respect context cancellation for graceful shutdown
//   - Emit events through the provided InvocationContext
//   - Handle the async resume mechanism properly
//   - Manage their lifecycle through Start/Stop methods
type Agent interface {
	Name() string
	Start(invocationCtx *InvocationContext) error
	Stop(invocationCtx *InvocationContext) error
	Run(invocationCtx *InvocationContext) error
	SetSubAgents(children ...Agent) error
	SubAgents() []Agent
	Parent() Agent
	FindAgent(name string) Agent
	Description() string
}

// AgentInfo carries identifying details about an agent used in contexts & events.
// Name is the external identifier; Type categorizes implementation (e.g. "orchestrator", "worker").
type AgentInfo struct{ Name, Type string }
