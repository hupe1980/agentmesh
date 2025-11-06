package graph

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	mermaid "github.com/hupe1980/agentmesh/internal/mermaid"
)

// Flowchart generates a Mermaid flowchart representation of the compiled graph.
//
//nolint:gocyclo // Flowchart generation requires handling many node and edge types
func (cg *CompiledGraph) Flowchart() string {
	chart := &mermaid.Flowchart{}

	nodes := make(map[string]struct{})
	for name := range cg.nodes {
		nodes[name] = struct{}{}
	}
	for _, edge := range cg.edges {
		if edge.From != "" {
			nodes[edge.From] = struct{}{}
		}
		if edge.To != "" {
			nodes[edge.To] = struct{}{}
		}
	}
	for _, ce := range cg.conditionals {
		if ce.From != "" {
			nodes[ce.From] = struct{}{}
		}
		for _, target := range ce.Targets {
			if target != "" {
				nodes[target] = struct{}{}
			}
		}
	}
	if len(nodes) == 0 {
		return chart.Render()
	}

	ordered := make([]string, 0, len(nodes))
	for name := range nodes {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	reserved := make(map[string]struct{}, len(ordered))
	idMap := make(map[string]string, len(ordered))
	for _, name := range ordered {
		id := sanitizeMermaidID(name, reserved)
		idMap[name] = id

		label := name
		shape := mermaid.ShapeDefault

		chart.AddNode(id, label, shape)
	}

	type edgeKey struct {
		from string
		to   string
	}

	solidSeen := make(map[edgeKey]struct{})

	for _, edge := range cg.edges {
		key := edgeKey{from: edge.From, to: edge.To}
		if _, exists := solidSeen[key]; exists {
			continue
		}
		fromID, okFrom := idMap[edge.From]
		toID, okTo := idMap[edge.To]
		if !okFrom || !okTo {
			continue
		}
		chart.AddEdge(fromID, toID, "")
		solidSeen[key] = struct{}{}
	}

	dashedSeen := make(map[edgeKey]struct{})
	for _, ce := range cg.conditionals {
		fromID, okFrom := idMap[ce.From]
		if !okFrom {
			continue
		}
		for _, target := range ce.Targets {
			key := edgeKey{from: ce.From, to: target}
			if _, exists := dashedSeen[key]; exists {
				continue
			}
			toID, okTo := idMap[target]
			if !okTo {
				continue
			}
			chart.AddEdgeWithStyle(fromID, toID, target, mermaid.EdgeDashed)
			dashedSeen[key] = struct{}{}
		}
	}

	return chart.Render()
}

func sanitizeMermaidID(name string, reserved map[string]struct{}) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "node"
	}

	var builder strings.Builder
	for _, r := range base {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune('_')
		case unicode.IsSpace(r):
			builder.WriteRune('_')
		default:
			builder.WriteRune('_')
		}
	}

	id := builder.String()
	if id == "" {
		id = "node"
	}
	if r := rune(id[0]); !unicode.IsLetter(r) && r != '_' {
		id = "n_" + id
	}

	candidate := id
	counter := 1
	for {
		if _, exists := reserved[candidate]; !exists {
			reserved[candidate] = struct{}{}
			return candidate
		}
		counter++
		candidate = fmt.Sprintf("%s_%d", id, counter)
	}
}
