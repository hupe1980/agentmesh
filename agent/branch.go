package agent

// buildBranchPath composes a hierarchical branch identifier used to isolate
// state mutations for child agents. If parent is empty it returns child;
// otherwise it returns parent + "." + child. An empty child returns parent.
func buildBranchPath(parent, child string) string {
	if parent == "" {
		return child
	}
	if child == "" { // defensive
		return parent
	}
	return parent + "." + child
}
