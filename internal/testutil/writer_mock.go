package testutil

import (
	"context"
	"sync"

	"github.com/hupe1980/agentmesh/core"
)

// CollectingWriter is an EventWriter that collects events in a slice for inspection.
type CollectingWriter struct {
	mu     sync.Mutex
	Events []core.Event
}

func (q *CollectingWriter) Write(_ context.Context, ev *core.Event) error {
	q.mu.Lock()
	q.Events = append(q.Events, *ev)
	q.mu.Unlock()
	return nil
}

// Compile-time assertion for interface conformance
var _ core.EventWriter = (*CollectingWriter)(nil)

// DiscardingWriter is an EventWriter that ignores all events.
type DiscardingWriter struct{}

func (DiscardingWriter) Write(_ context.Context, _ *core.Event) error { return nil }

// Compile-time assertion for interface conformance
var _ core.EventWriter = (*DiscardingWriter)(nil)
