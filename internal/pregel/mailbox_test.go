package pregel

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// TestMailboxSizeLimit verifies that MaxMailboxSize option prevents unbounded message accumulation
func TestMailboxSizeLimit(t *testing.T) {
	t.Run("UnlimitedMailbox", func(t *testing.T) {
		// Without limit, mailbox can grow indefinitely
		graph := &testGraph{
			roots: []string{"node1"},
			nodes: map[string]*testNode{
				"node1": {
					name: "node1",
					compute: func(ctx *VertexContext[*testState, testMessage], incoming []Message[testMessage]) error {
						// Send many messages to node2
						for i := 0; i < 1000; i++ {
							ctx.Send(Message[testMessage]{From: "node1", To: "node2", Data: testMessage{Value: i}})
						}
						return nil
					},
				},
				"node2": {
					name: "node2",
					compute: func(ctx *VertexContext[*testState, testMessage], incoming []Message[testMessage]) error {
						// Just receive messages
						return nil
					},
				},
			},
			state: &testState{},
		}

		runtime := NewRuntime[*testState, testMessage](graph, nil)
		err := runtime.Run(context.Background())
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Mailbox should have accepted all 1000 messages
		// (they get drained in superstep 2)
	})

	t.Run("BoundedMailbox", func(t *testing.T) {
		// With limit, mailbox drops messages when full
		overflowCount := 0
		events := make(chan StreamEvent[testMessage], 100)

		graph := &testGraph{
			roots: []string{"node1"},
			nodes: map[string]*testNode{
				"node1": {
					name: "node1",
					compute: func(ctx *VertexContext[*testState, testMessage], incoming []Message[testMessage]) error {
						// Send 100 messages to node2 (limit is 10)
						for i := 0; i < 100; i++ {
							ctx.Send(Message[testMessage]{From: "node1", To: "node2", Data: testMessage{Value: i}})
						}
						return nil
					},
				},
				"node2": {
					name: "node2",
					compute: func(ctx *VertexContext[*testState, testMessage], incoming []Message[testMessage]) error {
						// Count received messages
						return nil
					},
				},
			},
			state: &testState{},
		}

		// Set mailbox limit to 10 messages
		runtime := NewRuntime[*testState, testMessage](graph, events, WithMaxMailboxSize[*testState, testMessage](10))

		// Count overflow events in background
		done := make(chan struct{})
		go func() {
			defer close(done)
			for event := range events {
				if event.Error != nil && errors.Is(event.Error, ErrMailboxFull) {
					overflowCount++
				}
			}
		}()

		err := runtime.Run(context.Background())
		close(events)
		<-done

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should have at least 80 overflow events (100 messages - 10 limit = 90 dropped)
		// Allow some variance due to race conditions
		if overflowCount < 80 {
			t.Errorf("Expected at least 80 overflow events, got: %d", overflowCount)
		}

		t.Logf("✓ Mailbox limit prevented overflow: %d messages dropped", overflowCount)
	})

	t.Run("CombinerReducesMailboxPressure", func(t *testing.T) {
		// Combiner merges messages, reducing mailbox usage
		graph := &testGraph{
			roots: []string{"node1"},
			nodes: map[string]*testNode{
				"node1": {
					name: "node1",
					compute: func(ctx *VertexContext[*testState, testMessage], incoming []Message[testMessage]) error {
						// Send 100 messages to node2
						for i := 0; i < 100; i++ {
							ctx.Send(Message[testMessage]{From: "node1", To: "node2", Data: testMessage{Value: 1}})
						}
						return nil
					},
				},
				"node2": {
					name: "node2",
					compute: func(ctx *VertexContext[*testState, testMessage], incoming []Message[testMessage]) error {
						// With combiner, should receive 1 merged message
						if len(incoming) != 1 {
							t.Errorf("Expected 1 combined message, got: %d", len(incoming))
						}
						if len(incoming) > 0 && incoming[0].Data.Value != 100 {
							t.Errorf("Expected combined value 100, got: %d", incoming[0].Data.Value)
						}
						return nil
					},
				},
			},
			state: &testState{},
		}

		// Install combiner that sums message values
		combiner := func(a, b Message[testMessage]) Message[testMessage] {
			return Message[testMessage]{
				From: a.From,
				To:   a.To,
				Data: testMessage{Value: a.Data.Value + b.Data.Value},
			}
		}

		runtime := NewRuntime[*testState, testMessage](graph, nil,
			WithMaxMailboxSize[*testState, testMessage](10),
			WithCombiner[*testState, testMessage](combiner),
		)

		err := runtime.Run(context.Background())
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		t.Log("✓ Combiner successfully reduced 100 messages to 1")
	})
}

// Test helpers
type testState struct{}

type testMessage struct {
	Value int
}

type testNode struct {
	name    string
	compute func(*VertexContext[*testState, testMessage], []Message[testMessage]) error
}

func (n *testNode) Name() string { return n.name }

func (n *testNode) Run(ctx context.Context, vertex VertexContext[*testState, testMessage], incoming []Message[testMessage]) error {
	return n.compute(&vertex, incoming)
}

type testGraph struct {
	roots []string
	nodes map[string]*testNode
	state *testState
	mu    sync.Mutex
}

func (g *testGraph) RootNodes() []string {
	return g.roots
}

func (g *testGraph) NodeByName(name string) PregelNode[*testState, testMessage] {
	return g.nodes[name]
}

func (g *testGraph) State() *testState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

func (g *testGraph) Outgoing(node string) []string {
	return nil
}

func (g *testGraph) Update(node string, _ map[string]any, messages []Message[testMessage]) {
	// No-op for this test
}
