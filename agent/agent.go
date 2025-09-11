package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// eventWriterFunc is a functional adapter implementing core.EventWriter.
type eventWriterFunc func(ctx context.Context, ev *core.Event) error

// Write implements the core.EventWriter interface for eventWriterFunc.
func (f eventWriterFunc) Write(ctx context.Context, ev *core.Event) error { return f(ctx, ev) }
