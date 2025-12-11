package tool

import (
	"context"
	"errors"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Toolset defines a collection of tools that can be managed together.
// The interface supports both static and dynamic tool discovery.
type Toolset interface {
	// ListTools returns available tools.
	// The scope parameter provides read access to the current graph state,
	// enabling context-aware tool selection.
	// If scope is nil, returns all available tools (static discovery).
	ListTools(ctx context.Context, scope graph.ReadOnlyScope) ([]Tool, error)

	// Close releases any resources held by the toolset.
	Close() error
}

// StaticToolset wraps a static list of tools.
type StaticToolset struct {
	tools []Tool
}

// NewStaticToolset creates a toolset from a static list of tools.
func NewStaticToolset(tools ...Tool) *StaticToolset {
	return &StaticToolset{tools: tools}
}

// ListTools returns the static list of tools.
// The scope parameter is ignored for static toolsets.
func (s *StaticToolset) ListTools(_ context.Context, _ graph.ReadOnlyScope) ([]Tool, error) {
	return s.tools, nil
}

// Close is a no-op for static toolsets.
func (s *StaticToolset) Close() error {
	return nil
}

// CompositeToolset combines multiple toolsets into one.
type CompositeToolset struct {
	toolsets []Toolset
}

// Combine creates a composite toolset from multiple toolsets.
func Combine(toolsets ...Toolset) *CompositeToolset {
	return &CompositeToolset{toolsets: toolsets}
}

// ListTools returns tools from all contained toolsets.
func (c *CompositeToolset) ListTools(ctx context.Context, scope graph.ReadOnlyScope) ([]Tool, error) {
	var allTools []Tool

	for _, ts := range c.toolsets {
		tools, err := ts.ListTools(ctx, scope)
		if err != nil {
			return nil, err
		}

		allTools = append(allTools, tools...)
	}

	return allTools, nil
}

// Close releases resources from all contained toolsets.
func (c *CompositeToolset) Close() error {
	var errs []error

	for _, ts := range c.toolsets {
		if err := ts.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
