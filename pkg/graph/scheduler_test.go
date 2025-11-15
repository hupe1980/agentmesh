package graph

import (
	"context"
	"testing"

	stateif "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/stretchr/testify/require"
)

func noopNode(name string) *Node {
	return &Node{
		Name: name,
		RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return nil, nil
		},
	}
}

func mustCompileGraph(t *testing.T, g *Graph) *compiledImpl {
	t.Helper()
	cg, err := g.Compile()
	require.NoError(t, err)
	return cg
}

func mustAddNode(t testing.TB, g *Graph, n *Node) {
	t.Helper()
	require.NoError(t, g.AddNode(n))
}

func TestVertexSchedulerReadyInitial(t *testing.T) {
	t.Parallel()

	state, err := NewStateManager(0)
	require.NoError(t, err)
	g, err := NewGraph(state)
	require.NoError(t, err)
	process1 := noopNode("process1")
	mustAddNode(t, g, process1)
	mustAddNode(t, g, noopNode("process2"))
	g.AddEdge(StartNode, "process1")
	g.AddEdge("process1", "process2")

	cg := mustCompileGraph(t, g)

	sched := newVertexScheduler(cg)
	cg.bootstrapScheduler(context.Background(), sched)

	require.Equal(t, []string{"process1"}, sched.Ready())
}

func TestVertexSchedulerOnCompletion(t *testing.T) {
	t.Parallel()

	state, err := NewStateManager(0)
	require.NoError(t, err)
	g, err := NewGraph(state)
	require.NoError(t, err)
	process1 := noopNode("process1")
	mustAddNode(t, g, process1)
	mustAddNode(t, g, noopNode("process2"))
	g.AddEdge(StartNode, "process1")
	g.AddEdge("process1", "process2")

	cg := mustCompileGraph(t, g)

	sched := newVertexScheduler(cg)
	cg.bootstrapScheduler(context.Background(), sched)

	sched.MarkExecuted("process1")
	next, err := sched.OnVertexCompleted(context.Background(), "process1")
	require.NoError(t, err)
	require.Equal(t, []string{"process2"}, next)
	require.Equal(t, []string{"process2"}, sched.Ready())
}

func TestVertexSchedulerBootstrapResume(t *testing.T) {
	t.Parallel()

	state, err := NewStateManager(0)
	require.NoError(t, err)
	g, err := NewGraph(state)
	require.NoError(t, err)
	process1 := noopNode("process1")
	mustAddNode(t, g, process1)
	mustAddNode(t, g, noopNode("process2"))
	g.AddEdge(StartNode, "process1")
	g.AddEdge("process1", "process2")

	cg := mustCompileGraph(t, g)
	cg.setCurrentSuperstep(0)
	cg.markCompleted("process1")

	sched := newVertexScheduler(cg)
	cg.bootstrapScheduler(context.Background(), sched)

	require.Equal(t, []string{"process2"}, sched.Ready())
}

func TestVertexSchedulerStartConditionals(t *testing.T) {
	t.Parallel()

	state, err := NewStateManager(0)
	require.NoError(t, err)
	state.Set("branch", "taskB") // Initialize branch value
	g, err := NewGraph(state)
	require.NoError(t, err)

	// Add an entry node that always executes
	mustAddNode(t, g, noopNode("entry"))
	g.AddEdge(StartNode, "entry")

	// taskA and taskB are ONLY reachable via conditional edges (gated)
	mustAddNode(t, g, noopNode("taskA"))
	mustAddNode(t, g, noopNode("taskB"))

	// Conditional edges from entry (not START) to choose between taskA and taskB
	g.AddConditionalEdges("entry", func(_ context.Context, gs stateif.Reader) []string {
		if gs == nil {
			return nil
		}
		if val := gs.Get("branch"); val != nil {
			if str, ok := val.(string); ok && str != "" {
				return []string{str}
			}
		}
		return nil
	}, []string{"taskA", "taskB"})

	cg := mustCompileGraph(t, g)

	sched := newVertexScheduler(cg)
	cg.bootstrapScheduler(context.Background(), sched)

	// Only entry should be ready initially (taskA and taskB are gated)
	require.Equal(t, []string{"entry"}, sched.Ready())
}

func TestVertexSchedulerSnapshot(t *testing.T) {
	t.Parallel()

	state, err := NewStateManager(0)
	require.NoError(t, err)
	g, err := NewGraph(state)
	require.NoError(t, err)
	mustAddNode(t, g, noopNode("a"))
	mustAddNode(t, g, noopNode("b"))
	g.AddEdge(StartNode, "a")
	g.AddEdge("a", "b")

	cg := mustCompileGraph(t, g)
	sched := newVertexScheduler(cg)
	snapshot := sched.Snapshot()

	require.Contains(t, snapshot, "a")
	require.Contains(t, snapshot, "b")

	// Verify snapshot is a copy by modifying it
	aCopy := snapshot["a"]
	aCopy.Executed = true
	snapshot["a"] = aCopy
	// Original should be unchanged
	require.False(t, sched.tracker.WasExecuted("a"))
}

func TestVertexSchedulerReset(t *testing.T) {
	t.Parallel()

	state, err := NewStateManager(0)
	require.NoError(t, err)
	g, err := NewGraph(state)
	require.NoError(t, err)
	mustAddNode(t, g, noopNode("a"))
	g.AddEdge(StartNode, "a")

	cg := mustCompileGraph(t, g)
	sched := newVertexScheduler(cg)
	sched.MarkExecuted("a")

	sched.Reset()
	snapshot := sched.Snapshot()
	require.False(t, snapshot["a"].Executed)
	require.True(t, snapshot["a"].TopologyReady)
}

func TestVertexSchedulerConditionalSelectionDedup(t *testing.T) {
	t.Parallel()

	state, err := NewStateManager(0)
	require.NoError(t, err)
	g, err := NewGraph(state)
	require.NoError(t, err)

	mustAddNode(t, g, noopNode("next"))
	mustAddNode(t, g, noopNode("selector"))
	g.AddEdge(StartNode, "selector")
	g.AddEdge("selector", "next")
	g.AddConditionalEdges("selector", func(_ context.Context, gs stateif.Reader) []string { return []string{"next"} }, []string{"next"})

	cg := mustCompileGraph(t, g)

	sched := newVertexScheduler(cg)
	cg.bootstrapScheduler(context.Background(), sched)

	next, err := sched.OnVertexCompleted(context.Background(), "selector")
	require.NoError(t, err)
	require.Equal(t, []string{"next"}, next)
}
