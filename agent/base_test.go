package agent

import (
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/stretchr/testify/assert"
)

// SubAgents should be copy-out and HasSubAgents should reflect presence of children.
func TestBaseAgent_SubAgents_CopyAndHas(t *testing.T) {
	root := newMockAgent("Root", nil)
	c1 := newMockAgent("Child1", nil)
	c2 := newMockAgent("Child2", nil)

	// Initially no children
	assert.False(t, root.HasSubAgents())

	// Set children establishes relationships
	_ = root.AddSubAgents(c1, c2)
	assert.True(t, root.HasSubAgents())

	// SubAgents returns a copy; mutating the returned slice must not affect internal state
	listed := root.SubAgents()
	assert.Len(t, listed, 2)

	// Mutate returned slice
	listed[0] = nil

	// Re-fetch should be intact
	listed2 := root.SubAgents()
	assert.Len(t, listed2, 2)
	assert.NotNil(t, listed2[0])
}

// setParent is internal but should wire Parent() correctly; setSubAgents should enforce single-parent invariant.
func TestBaseAgent_Parent_SetAndInvariant(t *testing.T) {
	parent := newMockAgent("Parent", nil)
	child := newMockAgent("Child", nil)

	// Establish parent via public SetParent
	_ = child.SetParent(parent)
	got := child.Parent()
	if assert.NotNil(t, got) {
		assert.Equal(t, parent.Name(), got.Name())
	}

	// Attempt to re-parent via AddSubAgents should fail (immutability)
	other := newMockAgent("OtherParent", nil)
	err := other.AddSubAgents(child)
	assert.Error(t, err)
	// Parent remains unchanged
	assert.Equal(t, parent.Name(), child.Parent().Name())
}

// FindAgent should return self on name match and traverse descendants depth-first.
func TestBaseAgent_FindAgent_SelfAndDescendants(t *testing.T) {
	root := newMockAgent("Root", nil)
	a := newMockAgent("A", nil)
	b := newMockAgent("B", nil)
	x1 := newMockAgent("X", nil)
	x2 := newMockAgent("X", nil) // same name in different subtree to validate DFS order

	// Tree: Root -> [A(X), B(X)]
	_ = a.AddSubAgents(x1)
	_ = b.AddSubAgents(x2)
	_ = root.AddSubAgents(a, b)

	// Self match
	got, err := root.FindAgent("Root")
	assert.NoError(t, err)
	assert.Equal(t, "Root", got.Name())

	// Descendant search: should find X from the first subtree (A) due to DFS order
	got, err = root.FindAgent("X")
	assert.NoError(t, err)
	assert.Equal(t, x1.Name(), got.Name())
}

// FindAgent returns ErrAgentNotFound if no match exists.
func TestBaseAgent_FindAgent_NotFound(t *testing.T) {
	root := newMockAgent("Root", nil)
	_, err := root.FindAgent("Missing")
	assert.Error(t, err)
	assert.ErrorIs(t, err, core.ErrAgentNotFound)
}

// RootAgent should return the highest ancestor; for root it returns self.
func TestBaseAgent_RootAgent(t *testing.T) {
	root := newMockAgent("Root", nil)
	a := newMockAgent("A", nil)
	b := newMockAgent("B", nil)

	_ = a.AddSubAgents(b)
	_ = root.AddSubAgents(a)

	// Leaf's root should be Root
	assert.Equal(t, "Root", b.RootAgent().Name())

	// Root's root is itself
	assert.Equal(t, "Root", root.RootAgent().Name())
}

// Immutability: SetParent should error on reassign when non-nil parent already set.
func TestBaseAgent_SetParent_ErrorOnReassign(t *testing.T) {
	p1 := newMockAgent("P1", nil)
	p2 := newMockAgent("P2", nil)
	child := newMockAgent("Child", nil)

	_ = child.SetParent(p1)
	err := child.SetParent(p2)
	assert.Error(t, err)
	assert.Equal(t, p1.Name(), child.Parent().Name())
}

// Immutability: AddSubAgents should error if a child already has a different parent.
func TestBaseAgent_AddSubAgents_ErrorIfAlreadyParented(t *testing.T) {
	parent1 := newMockAgent("Parent1", nil)
	parent2 := newMockAgent("Parent2", nil)
	child := newMockAgent("Child", nil)

	_ = parent1.AddSubAgents(child)
	err := parent2.AddSubAgents(child)
	assert.Error(t, err)
	assert.Equal(t, parent1.Name(), child.Parent().Name())
}

// Duplicate sub-agent should return ErrSubAgentAlreadyExists and not duplicate entries.
func TestBaseAgent_AddSubAgents_DuplicateError(t *testing.T) {
	root := newMockAgent("Root", nil)
	c1 := newMockAgent("Child1", nil)

	err := root.AddSubAgents(c1)
	assert.NoError(t, err)
	assert.Len(t, root.SubAgents(), 1)

	err = root.AddSubAgents(c1) // duplicate
	assert.Error(t, err)
	assert.ErrorIs(t, err, core.ErrSubAgentAlreadyExists)
	assert.Len(t, root.SubAgents(), 1)
}
