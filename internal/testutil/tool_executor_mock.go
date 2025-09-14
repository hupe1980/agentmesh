package testutil

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// ToolExecutorMock implements core.ToolExecutor for tests, producing a simple
// function response event per incoming tool/function call. It mirrors the
// previous inline fakeExec used in flow/base_test.go.
type ToolExecutorMock struct {
	// TransferTo, if set, will be applied to each generated event's Actions.TransferToAgent.
	TransferTo core.Opt[string]
	// Err, if set, will be returned (and no events produced) when Execute is called.
	Err error
	// Events, if non-nil, will be returned verbatim (after applying TransferTo if set).
	Events []*core.Event
	// Calls counts how many times Execute was invoked.
	Calls int
}

func NewToolExecutorMock() *ToolExecutorMock { return &ToolExecutorMock{} }

// Execute builds one function response event per provided function call unless
// a predefined Events slice or Err is configured.
func (m *ToolExecutorMock) Execute(
	ctx context.Context,
	reqCtx core.RequestContext,
	toolRegistry map[string]core.Tool,
	fnCalls []*core.FunctionCall,
) ([]*core.Event, error) {
	m.Calls++
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Events != nil {
		if m.TransferTo.IsSet() {
			for _, ev := range m.Events {
				if !ev.Actions.TransferToAgent.IsSet() { // don't override if already populated
					ev.Actions.TransferToAgent = m.TransferTo
				}
			}
		}
		return m.Events, nil
	}
	events := make([]*core.Event, 0, len(fnCalls))
	for _, c := range fnCalls {
		ev := core.NewFunctionResponseEvent(
			reqCtx.RunID(),
			reqCtx.AgentName(),
			c.ID,
			c.Name,
			map[string]any{"ok": true},
		)
		ev.Actions.TransferToAgent = m.TransferTo
		events = append(events, ev)
	}
	return events, nil
}
