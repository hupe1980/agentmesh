package mermaid

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlowchartRender(t *testing.T) {
	fc := NewFlowchart(WithDirection("LR"))

	fc.AddNode("A", "Start", ShapeDefault)
	fc.AddNode("B", "Process", ShapeRounded)
	fc.AddNode("C", "Tool node", ShapeSubroutine)

	fc.AddEdge("A", "B", "next")
	fc.AddEdgeWithStyle("B", "C", "uses", EdgeDashed)
	fc.AddEdge("C", "A", "")

	chart := fc.Render()

	require.Contains(t, chart, "flowchart LR")
	require.Contains(t, chart, "A[\"Start\"]")
	require.Contains(t, chart, "B(\"Process\")")
	require.Contains(t, chart, "C[[\"Tool node\"]]")
	require.Contains(t, chart, "A -->|next| B")
	require.Contains(t, chart, "B -.->|uses| C")
	require.Contains(t, chart, "C --> A")
}

func TestFlowchartEscaping(t *testing.T) {
	fc := NewFlowchart()
	fc.AddNode("node", "multi\nline \"label\"", ShapeDefault)
	fc.AddEdge("node", "node", "edge \"label\"")

	chart := fc.Render()

	require.Contains(t, chart, "multi<br/>line \\\"label\\\"")
	require.Contains(t, chart, "node -->|edge \\\"label\\\"| node")
}
