package mermaid

import (
	"fmt"
	"strings"
)

// FlowchartOption configures Flowchart rendering behaviour.
type FlowchartOption func(*Flowchart)

// Flowchart represents a Mermaid flowchart under construction.
type Flowchart struct {
	direction string
	nodes     map[string]node
	nodeOrder []string
	edges     []edge
}

type node struct {
	id    string
	label string
	shape NodeShape
}

type edge struct {
	from  string
	to    string
	label string
	style EdgeStyle
}

// NodeShape controls how a node is rendered in Mermaid syntax.
type NodeShape struct {
	left  string
	right string
}

// EdgeStyle controls the visual style of an edge in Mermaid syntax.
type EdgeStyle int

const (
	// EdgeSolid renders a standard solid arrow (-->)
	EdgeSolid EdgeStyle = iota
	// EdgeDashed renders a dashed arrow (-.->)
	EdgeDashed
)

var (
	// ShapeDefault renders nodes as standard rectangles: id["label"].
	ShapeDefault = NodeShape{left: "[\"", right: "\"]"}
	// ShapeRounded renders nodes with rounded edges: id("label").
	ShapeRounded = NodeShape{left: "(\"", right: "\")"}
	// ShapeStadium renders nodes as stadiums: id(["label"]).
	ShapeStadium = NodeShape{left: "([\"", right: "\"])"}
	// ShapeSubroutine renders nodes with double-stroke borders: id[["label"]].
	ShapeSubroutine = NodeShape{left: "[[\"", right: "\"]]"}
)

// NewFlowchart constructs a Flowchart with optional configuration.
func NewFlowchart(optFns ...FlowchartOption) *Flowchart {
	fc := &Flowchart{
		direction: "TD",
		nodes:     make(map[string]node),
		nodeOrder: make([]string, 0),
		edges:     make([]edge, 0),
	}

	for _, fn := range optFns {
		if fn != nil {
			fn(fc)
		}
	}

	return fc
}

// WithDirection sets the layout direction of the rendered flowchart (e.g. TD, LR, BT, RL).
func WithDirection(direction string) FlowchartOption {
	direction = strings.TrimSpace(direction)
	if direction == "" {
		return func(*Flowchart) {}
	}

	direction = strings.ToUpper(direction)

	return func(fc *Flowchart) {
		fc.direction = direction
	}
}

// AddNode registers or updates a node in the flowchart. Nodes are rendered in
// insertion order. When a node ID already exists, its label and shape are
// updated with the latest values.
func (fc *Flowchart) AddNode(id, label string, shape NodeShape) {
	if id == "" {
		return
	}

	if shape.left == "" && shape.right == "" {
		shape = ShapeDefault
	}

	if _, exists := fc.nodes[id]; !exists {
		fc.nodeOrder = append(fc.nodeOrder, id)
	}

	fc.nodes[id] = node{
		id:    id,
		label: label,
		shape: shape,
	}
}

// AddEdge registers a directed edge between two nodes. Edges are rendered in
// insertion order. The provided node IDs must already exist in the flowchart in
// order to produce a meaningful diagram, though this method does not enforce
// that constraint.
func (fc *Flowchart) AddEdge(from, to, label string) {
	fc.AddEdgeWithStyle(from, to, label, EdgeSolid)
}

// AddEdgeWithStyle registers a directed edge between two nodes using the provided style.
func (fc *Flowchart) AddEdgeWithStyle(from, to, label string, style EdgeStyle) {
	if from == "" || to == "" {
		return
	}

	fc.edges = append(fc.edges, edge{from: from, to: to, label: label, style: style})
}

// Render produces the Mermaid flowchart representation.
func (fc *Flowchart) Render() string {
	var out strings.Builder

	fmt.Fprintf(&out, "flowchart %s\n", fc.direction)

	for _, id := range fc.nodeOrder {
		n := fc.nodes[id]
		out.WriteString("    ")
		out.WriteString(n.id)
		out.WriteString(n.shape.left)
		out.WriteString(escapeLabel(n.label))
		out.WriteString(n.shape.right)
		out.WriteByte('\n')
	}

	for _, e := range fc.edges {
		out.WriteString("    ")
		out.WriteString(e.from)
		out.WriteString(" ")

		arrow := "-->"
		if e.style == EdgeDashed {
			arrow = "-.->"
		}

		if trimmed := strings.TrimSpace(e.label); trimmed != "" {
			out.WriteString(strings.TrimSuffix(arrow, ">"))
			out.WriteString(">|")
			out.WriteString(escapeEdgeLabel(trimmed))
			out.WriteString("|")
		} else {
			out.WriteString(arrow)
		}
		out.WriteByte(' ')
		out.WriteString(e.to)
		out.WriteByte('\n')
	}

	return out.String()
}

func escapeLabel(value string) string {
	if value == "" {
		return ""
	}

	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "<br/>")

	return value
}

func escapeEdgeLabel(value string) string {
	if value == "" {
		return ""
	}

	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", " ")

	return value
}
