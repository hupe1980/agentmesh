package agent

import "github.com/hupe1980/agentmesh/pkg/graph"

// GraphProvider exposes the underlying graph structure
type GraphProvider interface {
	Graph() *graph.Graph
}

// TopologyProvider exposes graph topology information
type TopologyProvider interface {
	GetTopology() *graph.Topology
	GetNodes() []string
}

// NodeIntrospector provides detailed node information
type NodeIntrospector interface {
	GetNodeInfo(name string) (*graph.NodeInfo, error)
	GetNodeDependencies(name string) (*graph.NodeDependencies, error)
}

// MetricsProvider exposes execution metrics
type MetricsProvider interface {
	GetMetrics() *graph.Metrics
}

// DiagramGenerator generates visual representations
type DiagramGenerator interface {
	MermaidFlowchart(direction string) string
}

// FullIntrospector combines all introspection capabilities
// This is typically implemented by graph-based runnables
type FullIntrospector interface {
	GraphProvider
	TopologyProvider
	NodeIntrospector
	MetricsProvider
	DiagramGenerator
}
