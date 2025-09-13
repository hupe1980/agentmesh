package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func TestNewParallelAgent(t *testing.T) {
	c1 := newMockAgent("Child1", func(context.Context, core.RequestContext, core.EventWriter) error { return nil })
	c2 := newMockAgent("Child2", func(context.Context, core.RequestContext, core.EventWriter) error { return nil })

	p := NewParallelAgent("ParallelAgent", []core.Agent{c1, c2})
	assert.Equal(t, "ParallelAgent", p.Name())
	assert.Len(t, p.SubAgents(), 2)
	assert.Same(t, c1, p.SubAgents()[0])
	assert.Same(t, c2, p.SubAgents()[1])
}

func TestParallelAgent_Run_Success(t *testing.T) {
	// Collect branches concurrently
	var mu sync.Mutex
	branches := map[string]string{}

	mkChild := func(name string) *mockAgent {
		return newMockAgent(name, func(_ context.Context, ctx core.RequestContext, _ core.EventWriter) error {
			mu.Lock()
			branches[name] = ctx.Branch()
			mu.Unlock()
			return nil
		})
	}

	c1 := mkChild("Child1")
	c2 := mkChild("Child2")
	c3 := mkChild("Child3")

	p := NewParallelAgent("ParallelAgent", []core.Agent{c1, c2, c3})
	reqCtx := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = p
	})

	err := p.Run(context.Background(), reqCtx, testutil.DiscardingWriter{})
	assert.NoError(t, err)

	// All children should have been executed with isolated cloned contexts.
	assert.Len(t, branches, 3)

	// Ensure each branch contains hierarchical naming pattern: ParentName.ChildName
	for _, child := range []*mockAgent{c1, c2, c3} {
		assert.NotNil(t, child.ReceivedCtx())
		b := child.ReceivedCtx().Branch()
		assert.Truef(t, strings.HasSuffix(b, "ParallelAgent."+child.Name()), "branch %s has correct suffix", b)
	}

	// Original request context branch should remain unchanged (empty)
	assert.Equal(t, "", reqCtx.Branch())
}

func TestParallelAgent_Run_ErrorAggregation(t *testing.T) {
	sentinel := errors.New("boom")

	c1 := newMockAgent(
		"Child1",
		func(context.Context, core.RequestContext, core.EventWriter) error { return nil },
	)
	c2 := newMockAgent(
		"Child2",
		func(context.Context, core.RequestContext, core.EventWriter) error { return sentinel },
	)
	c3 := newMockAgent(
		"Child3",
		func(context.Context, core.RequestContext, core.EventWriter) error { return nil },
	)

	p := NewParallelAgent("ParallelAgent", []core.Agent{c1, c2, c3})
	reqCtx := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = p
	})

	err := p.Run(context.Background(), reqCtx, testutil.DiscardingWriter{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	// Current implementation includes agent ID not name in error text.
	assert.Contains(t, err.Error(), "agent Child2")

	// All children should have executed despite an error (error returned after wait)
	assert.NotNil(t, c1.ReceivedCtx())
	assert.NotNil(t, c2.ReceivedCtx())
	assert.NotNil(t, c3.ReceivedCtx())
}

func TestParallelAgent_Run_NoChildren(t *testing.T) {
	p := NewParallelAgent("ParallelAgent", []core.Agent{})
	reqCtx := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = p
	})
	err := p.Run(context.Background(), reqCtx, testutil.DiscardingWriter{})
	assert.NoError(t, err)
}

func TestParallelAgent_Run_TimeoutWrap(t *testing.T) {
	child := newMockAgent("Slow", func(ctx context.Context, _ core.RequestContext, _ core.EventWriter) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return nil
		}
	})
	p := NewParallelAgent("P", []core.Agent{child}, func(o *ParallelAgentOptions) {
		o.Timeout = 10 * time.Millisecond
	})
	reqCtx := testutil.NewTestRequestContext(func(rcp *core.RequestContextParams) {
		rcp.Agent = p
	})

	start := time.Now()
	err := p.Run(context.Background(), reqCtx, testutil.DiscardingWriter{})
	dur := time.Since(start)

	assert.Error(t, err)
	assert.ErrorIs(t, err, core.ErrParallelTimeout)
	assert.Less(t, dur, 50*time.Millisecond)
}

// BaseAgent hierarchy tests (focus on SetSubAgents & FindAgent behavior)
func TestBaseAgent_SetSubAgentsAndFind(t *testing.T) {
	root := newMockAgent("Root", nil)
	c1 := newMockAgent("Child1", nil)
	c2 := newMockAgent("Child2", nil)

	// Establish children
	_ = root.AddSubAgents(c1, c2)
	subs := root.SubAgents()
	assert.Len(t, subs, 2)

	// Parents set
	assert.NotNil(t, c1.Parent())
	assert.Equal(t, root.Name(), c1.Parent().Name())
	assert.NotNil(t, c2.Parent())

	// Find direct child
	foundChild, err := root.FindAgent("Child1")
	assert.NoError(t, err)
	assert.NotNil(t, foundChild)
	assert.Equal(t, c1.Name(), foundChild.Name())

	// Find self
	foundRoot, err := root.FindAgent("Root")
	assert.NoError(t, err)
	assert.NotNil(t, foundRoot)
}

func TestBaseAgent_AddSubAgents_ReassignDenied(t *testing.T) {
	root := newMockAgent("Root", nil)
	c1 := newMockAgent("Child1", nil)
	c2 := newMockAgent("Child2", nil)
	c3 := newMockAgent("Child3", nil)

	_ = root.AddSubAgents(c1, c2)
	// Attempt to forcibly clear and reassign should not change existing parent of c1,c2
	// and adding c3 still works (appending) since it's new.
	err := root.AddSubAgents(c3)
	assert.NoError(t, err)
	assert.Equal(t, root.Name(), c3.Parent().Name())
	assert.Equal(t, root.Name(), c1.Parent().Name())
	assert.Equal(t, 3, len(root.SubAgents()))
}
