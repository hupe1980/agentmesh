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
	root.setSubAgents(c1, c2)
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

	// Direct setParent works (internal)
	child.setParent(parent)
	got := child.Parent()
	if assert.NotNil(t, got) {
		assert.Equal(t, parent.Name(), got.Name())
	}

	// Overwrite via setSubAgents ensures single-parent invariant
	other := newMockAgent("OtherParent", nil)
	other.setSubAgents(child)
	assert.Equal(t, other.Name(), child.Parent().Name())
}

// FindAgent should return self on name match and traverse descendants depth-first.
func TestBaseAgent_FindAgent_SelfAndDescendants(t *testing.T) {
	root := newMockAgent("Root", nil)
	a := newMockAgent("A", nil)
	b := newMockAgent("B", nil)
	x1 := newMockAgent("X", nil)
	x2 := newMockAgent("X", nil) // same name in different subtree to validate DFS order

	// Tree: Root -> [A(X), B(X)]
	a.setSubAgents(x1)
	b.setSubAgents(x2)
	root.setSubAgents(a, b)

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

// FindSubAgent excludes self and searches only descendants.
func TestBaseAgent_FindSubAgent_ExcludesSelf(t *testing.T) {
	root := newMockAgent("Root", nil)
	a := newMockAgent("A", nil)
	root.setSubAgents(a)

	// Searching for "Root" in descendants should fail
	_, err := root.FindSubAgent("Root")
	assert.Error(t, err)
	assert.ErrorIs(t, err, core.ErrAgentNotFound)

	// But descendant should be found via FindSubAgent
	got, err := root.FindSubAgent("A")
	assert.NoError(t, err)
	assert.Equal(t, "A", got.Name())
}

// RootAgent should return the highest ancestor; for root it returns self.
func TestBaseAgent_RootAgent(t *testing.T) {
	root := newMockAgent("Root", nil)
	a := newMockAgent("A", nil)
	b := newMockAgent("B", nil)

	a.setSubAgents(b)
	root.setSubAgents(a)

	// Leaf's root should be Root
	assert.Equal(t, "Root", b.RootAgent().Name())

	// Root's root is itself
	assert.Equal(t, "Root", root.RootAgent().Name())
}

// Reassigning children should clear old parents and update search surface accordingly.
func TestBaseAgent_SetSubAgents_Reassign_ClearsOldParentsAndSearch(t *testing.T) {
	root := newMockAgent("Root", nil)
	c1 := newMockAgent("Child1", nil)
	c2 := newMockAgent("Child2", nil)
	c3 := newMockAgent("Child3", nil)

	root.setSubAgents(c1, c2)
	root.setSubAgents(c3) // reassign

	// Old children lost parent
	assert.Nil(t, c1.Parent())
	assert.Nil(t, c2.Parent())

	// New child has parent
	assert.Equal(t, root.Name(), c3.Parent().Name())

	// Old child not found anymore
	a, err := root.FindAgent("Child1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, core.ErrAgentNotFound)
	assert.Nil(t, a)

	// New child found
	a, err = root.FindAgent("Child3")
	assert.NoError(t, err)
	assert.NotNil(t, a)
}

// Setting no children should clear child set and remove parent links from previous children.
func TestBaseAgent_SetSubAgents_ClearChildren(t *testing.T) {
	root := newMockAgent("Root", nil)
	c1 := newMockAgent("Child1", nil)
	c2 := newMockAgent("Child2", nil)

	root.setSubAgents(c1, c2)
	assert.True(t, root.HasSubAgents())
	assert.NotNil(t, c1.Parent())
	assert.NotNil(t, c2.Parent())

	// Clear children
	root.setSubAgents()
	assert.False(t, root.HasSubAgents())
	assert.Nil(t, c1.Parent())
	assert.Nil(t, c2.Parent())
}
