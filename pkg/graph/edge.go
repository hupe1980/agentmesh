package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Edge represents a directed connection between two nodes.
type Edge struct {
	From string // Source node name
	To   string // Target node name
}

// ConditionalEdges represents dynamic routing based on state.
type ConditionalEdges struct {
	From      string                                          // Source node
	Condition func(context.Context, *state.ReadView) []string // Returns target node names
	Targets   []string                                        // All possible targets
}
