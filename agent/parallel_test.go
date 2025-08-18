package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/stretchr/testify/assert"
)

// testChildAgent is a lightweight concrete agent used for testing composite agents.
// It captures the invocation context passed to Run and optionally returns an error.
type testChildAgent struct {
	BaseAgent
	runFn       func(*core.InvocationContext) error
	receivedCtx *core.InvocationContext
}

func newTestChildAgent(name string, runFn func(*core.InvocationContext) error) *testChildAgent {
	if runFn == nil {
		runFn = func(*core.InvocationContext) error { return nil }
	}

	return &testChildAgent{BaseAgent: NewBaseAgent(name), runFn: runFn}
}

func (t *testChildAgent) Run(invocationCtx *core.InvocationContext) error {
	t.receivedCtx = invocationCtx
	return t.runFn(invocationCtx)
}

// ParallelAgent Tests
func TestNewParallelAgent(t *testing.T) {
	c1 := newTestChildAgent("Child1", nil)
	c2 := newTestChildAgent("Child2", nil)

	p := NewParallelAgent("ParallelAgent", 0, c1, c2)
	assert.Equal(t, "ParallelAgent", p.Name())
	assert.Len(t, p.children, 2)
	assert.Same(t, c1, p.children[0])
	assert.Same(t, c2, p.children[1])
}

func makeInvocationCtx(t *testing.T, agentID, agentName, agentType string) *core.InvocationContext {
	t.Helper()
	emit := make(chan core.Event, 10)
	resume := make(chan struct{}, 1)
	userContent := core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "hello"}}}
	info := core.AgentInfo{Name: agentName, Type: agentType}
	return core.NewInvocationContext(context.Background(), "session-1", "inv-1", info, userContent, emit, resume, core.NewSession("session-1"), nil, nil, nil, logging.NoOpLogger{})
}

func TestParallelAgent_Run_Success(t *testing.T) {
	// Collect branches concurrently
	var mu sync.Mutex
	branches := map[string]string{}

	mkChild := func(name string) *testChildAgent {
		return newTestChildAgent(name, func(ctx *core.InvocationContext) error {
			mu.Lock()
			branches[name] = ctx.Branch
			mu.Unlock()
			return nil
		})
	}

	c1 := mkChild("Child1")
	c2 := mkChild("Child2")
	c3 := mkChild("Child3")

	p := NewParallelAgent("ParallelAgent", 0, c1, c2, c3)

	invCtx := makeInvocationCtx(t, "p1", "ParallelAgent", "parallel")

	err := p.Run(invCtx)
	assert.NoError(t, err)

	// All children should have been executed with isolated cloned contexts.
	assert.Len(t, branches, 3)

	// Ensure each branch contains hierarchical naming pattern: ParentName.ChildName
	for _, child := range []*testChildAgent{c1, c2, c3} {
		assert.NotNil(t, child.receivedCtx)
		assert.Truef(t, strings.HasSuffix(child.receivedCtx.Branch, "ParallelAgent."+child.Name()), "branch %s has correct suffix", child.receivedCtx.Branch)
	}

	// Original invocation context branch should remain unchanged (empty)
	assert.Equal(t, "", invCtx.Branch)
}

func TestParallelAgent_Run_ErrorAggregation(t *testing.T) {
	sentinel := errors.New("boom")

	c1 := newTestChildAgent("Child1", func(_ *core.InvocationContext) error { return nil })
	c2 := newTestChildAgent("Child2", func(_ *core.InvocationContext) error { return sentinel })
	c3 := newTestChildAgent("Child3", func(_ *core.InvocationContext) error { return nil })

	p := NewParallelAgent("ParallelAgent", 0, c1, c2, c3)
	invCtx := makeInvocationCtx(t, "p1", "ParallelAgent", "parallel")

	err := p.Run(invCtx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	// Current implementation includes agent ID not name in error text.
	assert.Contains(t, err.Error(), "agent Child2")

	// All children should have executed despite an error (error returned after wait)
	assert.NotNil(t, c1.receivedCtx)
	assert.NotNil(t, c2.receivedCtx)
	assert.NotNil(t, c3.receivedCtx)
}

func TestParallelAgent_Run_NoChildren(t *testing.T) {
	p := NewParallelAgent("ParallelAgent", 0)
	invCtx := makeInvocationCtx(t, "p1", "ParallelAgent", "parallel")
	err := p.Run(invCtx)
	assert.NoError(t, err)
}

// BaseAgent hierarchy tests (focus on SetSubAgents & FindAgent behavior)
func TestBaseAgent_SetSubAgentsAndFind(t *testing.T) {
	root := newTestChildAgent("Root", nil)
	c1 := newTestChildAgent("Child1", nil)
	c2 := newTestChildAgent("Child2", nil)

	// Establish children
	err := root.SetSubAgents(c1, c2)
	assert.NoError(t, err)
	subs := root.SubAgents()
	assert.Len(t, subs, 2)

	// Parents set
	assert.NotNil(t, c1.Parent())
	assert.Equal(t, root.Name(), c1.Parent().Name())
	assert.NotNil(t, c2.Parent())

	// Find direct child
	foundChild := root.FindAgent("Child1")
	assert.NotNil(t, foundChild)
	assert.Equal(t, c1.Name(), foundChild.Name())

	// Find self returns wrapper (not the same pointer)
	foundRoot := root.FindAgent("Root")
	assert.NotNil(t, foundRoot)
	assert.Equal(t, root.Name(), foundRoot.Name())
	if foundRoot == root { // This could change if implementation changes
		t.Log("FindAgent returned the root directly (wrapper semantics may have changed)")
	}
}

func TestBaseAgent_SetSubAgents_ReassignClearsOldParents(t *testing.T) {
	root := newTestChildAgent("Root", nil)
	c1 := newTestChildAgent("Child1", nil)
	c2 := newTestChildAgent("Child2", nil)
	c3 := newTestChildAgent("Child3", nil)

	assert.NoError(t, root.SetSubAgents(c1, c2))
	assert.NoError(t, root.SetSubAgents(c3)) // reassign

	// Old children lost parent
	assert.Nil(t, c1.Parent())
	assert.Nil(t, c2.Parent())

	// New child has parent
	assert.Equal(t, root.Name(), c3.Parent().Name())

	// Old child not found anymore
	assert.Nil(t, root.FindAgent("Child1"))
	// New child found
	assert.NotNil(t, root.FindAgent("Child3"))
}
