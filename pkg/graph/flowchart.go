package graph

// Flowchart generates a Mermaid flowchart representation of the compiled graph.
// This is a convenience method that calls GenerateMermaidFlowchart with default "TD" direction.
//
// Deprecated: Use GenerateMermaidFlowchart for more control over the output format.
func (cg *CompiledGraph) Flowchart() string {
	return cg.GenerateMermaidFlowchart("TD")
}
