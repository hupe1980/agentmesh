package agent

import (
	"errors"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// NewAgent creates a new agent graph with the specified model and tools.
// It builds a graph with a model node that can invoke tools and a tool execution node.
func NewAgent(name string, mdl model.Model, tools ...tool.Tool) (*graph.CompiledGraph, error) {
	toolRegistry := make(map[string]tool.Tool, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		toolRegistry[t.Name()] = t
	}

	acceptedTools := make([]tool.Tool, 0, len(toolRegistry))
	for _, t := range toolRegistry {
		acceptedTools = append(acceptedTools, t)
	}

	if toolAware, ok := mdl.(model.ToolAware); ok {
		configured := toolAware.BindTools(acceptedTools...)
		if configured == nil {
			return nil, errors.New("agent: model returned nil from BindTools")
		}
		mdl = configured
	} else if len(acceptedTools) > 0 {
		return nil, errors.New("agent: model does not support tool configuration")
	}

	// Create state with unlimited messages (0 = unlimited)
	state := graph.NewGraphState(0)

	g := graph.NewGraph(state)

	_ = g.AddNode(ModelNode(mdl))

	_ = g.AddNode(ToolNode(toolRegistry, WithToolErrorPrefix("agent")))

	// Routing: check if last message has tool calls
	g.AddEdge(graph.StartNode, "model")

	g.AddConditionalEdges("model", RouteOnToolCalls("tool", graph.EndNode), []string{"tool", graph.EndNode})

	g.AddEdge("tool", "model")

	return g.Compile()
}
