package graph

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVertexSchedulerRaceSafety(t *testing.T) {
	if testing.Short() {
		t.Skip("skip race guard in short mode")
	}

	state, err := NewStateManager(0)
	require.NoError(t, err)
	g, err := NewGraph(state)
	require.NoError(t, err)
	require.NoError(t, g.AddNode(noopNode("start")))
	require.NoError(t, g.AddNode(noopNode("next")))
	g.AddEdge(StartNode, "start")
	g.AddEdge("start", "next")

	cg := mustCompileGraph(t, g)
	sched := newVertexScheduler(cg)
	cg.bootstrapScheduler(context.Background(), sched)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		sched.MarkExecuted("start")
		_, _ = sched.OnVertexCompleted(context.Background(), "start")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = sched.Ready()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = sched.EnsureVertexExists("next")
	}()

	wg.Wait()
}
