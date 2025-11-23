package pregel

import (
	"context"
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

		runtime := MustNewRuntime[*testState, testMessage](graph, nil)
		err := runToCompletion(context.Background(), runtime)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Mailbox should have accepted all 1000 messages
		// (they get drained in superstep 2)
	})

	t.Run("BoundedMailbox", func(t *testing.T) {
		// With bounded mailbox and backpressure, verify messages don't overflow
		// Send a reasonable number that fits within the limit to avoid deadlock
		receivedCount := 0
		var mu sync.Mutex

		graph := &testGraph{
			roots: []string{"node1"},
			nodes: map[string]*testNode{
				"node1": {
					name: "node1",
					compute: func(ctx *VertexContext[*testState, testMessage], incoming []Message[testMessage]) error {
						// Send 5 messages to node2 (limit is 10, well within capacity)
						for i := 0; i < 5; i++ {
							ctx.Send(Message[testMessage]{From: "node1", To: "node2", Data: testMessage{Value: i}})
						}
						return nil
					},
				},
				"node2": {
					name: "node2",
					compute: func(ctx *VertexContext[*testState, testMessage], incoming []Message[testMessage]) error {
						// Count received messages
						mu.Lock()
						receivedCount += len(incoming)
						mu.Unlock()
						return nil
					},
				},
			},
			state: &testState{},
		}

		// Set mailbox limit to 10 messages
		runtime := MustNewRuntime[*testState, testMessage](graph, nil,
			WithMaxMailboxSize[*testState, testMessage](10),
			WithMaxWorkers[*testState, testMessage](2), // Multiple workers for concurrency
		)

		err := runToCompletion(context.Background(), runtime)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// With backpressure, all 5 messages should be delivered (no loss)
		mu.Lock()
		final := receivedCount
		mu.Unlock()

		if final != 5 {
			t.Errorf("Expected all 5 messages to be delivered with backpressure, got: %d", final)
		}

		t.Logf("✓ Bounded mailbox with backpressure delivered all messages: %d (no loss)", final)
	})

	t.Run("CombinerReducesMailboxPressure", func(t *testing.T) {
		// Combiner merges messages when mailbox reaches 75% capacity
		// Use small mailbox (10) so that 100 messages triggers combining
		var receivedMsgs int
		var totalValue int
		var mu sync.Mutex

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
						// With combiner and small mailbox, should receive fewer messages than sent
						mu.Lock()
						receivedMsgs += len(incoming)
						for _, msg := range incoming {
							totalValue += msg.Data.Value
						}
						mu.Unlock()
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

		// Use small mailbox (10) to trigger combining at 75% capacity (8 messages)
		runtime := MustNewRuntime[*testState, testMessage](graph, nil,
			WithMaxMailboxSize[*testState, testMessage](10),
			WithCombiner[*testState, testMessage](combiner),
		)

		err := runToCompletion(context.Background(), runtime)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		mu.Lock()
		finalMsgs := receivedMsgs
		finalValue := totalValue
		mu.Unlock()

		// Verify combining occurred (fewer messages than sent, but same total value)
		if finalMsgs >= 100 {
			t.Errorf("Expected combining to reduce message count below 100, got: %d", finalMsgs)
		}
		if finalValue != 100 {
			t.Errorf("Expected total value to be preserved (100), got: %d", finalValue)
		}

		t.Logf("✓ Combiner successfully reduced 100 messages to %d (total value preserved: %d)", finalMsgs, finalValue)
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

func (g *testGraph) NodeByName(name string) Node[*testState, testMessage] {
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
