package compile

// NodeInfo contains metadata about a node in the graph.
type NodeInfo struct {
	Name              string `json:"name"`
	Type              string `json:"type"` // "standard", "start", "end"
	IncomingEdges     int    `json:"incoming_edges"`
	OutgoingEdges     int    `json:"outgoing_edges"`
	IsConditional     bool   `json:"is_conditional"`
	IsConditionalGate bool   `json:"is_conditional_gate"`
	HasRetryPolicy    bool   `json:"has_retry_policy"`
	RetryMaxAttempts  int    `json:"retry_max_attempts,omitempty"`
}

// EdgeInfo contains metadata about an edge in the graph.
type EdgeInfo struct {
	From               string   `json:"from"`
	To                 string   `json:"to"`
	Type               string   `json:"type"` // "direct", "conditional"
	ConditionalTargets []string `json:"conditional_targets,omitempty"`
}

// Topology provides a complete introspection view of the compiled graph structure.
// This is the public-facing API for graph analysis and visualization.
type Topology struct {
	Nodes            []NodeInfo `json:"nodes"`
	Edges            []EdgeInfo `json:"edges"`
	EntryPoints      []string   `json:"entry_points"`
	ExitPoints       []string   `json:"exit_points"`
	ConditionalNodes []string   `json:"conditional_nodes"`
	IsolatedNodes    []string   `json:"isolated_nodes"`
	MaxDepth         int        `json:"max_depth"`
	TotalPaths       int        `json:"total_paths"`
	TotalNodes       int        `json:"total_nodes"`
	TotalEdges       int        `json:"total_edges"`
}

// Metrics provides runtime execution metrics.
type Metrics struct {
	TotalNodes           int            `json:"total_nodes"`
	TotalEdges           int            `json:"total_edges"`
	ConditionalEdges     int            `json:"conditional_edges"`
	AverageFanOut        float64        `json:"average_fan_out"`
	MaxFanOut            int            `json:"max_fan_out"`
	AverageFanIn         float64        `json:"average_fan_in"`
	MaxFanIn             int            `json:"max_fan_in"`
	CyclomaticComplexity int            `json:"cyclomatic_complexity"`
	NodesByType          map[string]int `json:"nodes_by_type"`
}

// NodeDependencies contains dependency information for a node.
type NodeDependencies struct {
	NodeName           string   `json:"node_name"`
	DirectPredecessors []string `json:"direct_predecessors"`
	DirectSuccessors   []string `json:"direct_successors"`
	AllPredecessors    []string `json:"all_predecessors"` // All nodes that must execute before this one
	AllSuccessors      []string `json:"all_successors"`   // All nodes that execute after this one
	Depth              int      `json:"depth"`            // Distance from START node
}
