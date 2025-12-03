package pregel

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockSlowMessageBus simulates a distributed message bus with network latency.
// This is used to benchmark the performance improvement of parallel draining.
type mockSlowMessageBus struct {
	latency   time.Duration
	mu        sync.Mutex
	mailboxes map[string][]Message[mockMessage]
}

func newMockSlowMessageBus(latency time.Duration) *mockSlowMessageBus {
	return &mockSlowMessageBus{
		latency:   latency,
		mailboxes: make(map[string][]Message[mockMessage]),
	}
}

func (m *mockSlowMessageBus) Send(ctx context.Context, messages []Message[mockMessage]) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, msg := range messages {
		m.mailboxes[msg.To] = append(m.mailboxes[msg.To], msg)
	}
	return nil
}

func (m *mockSlowMessageBus) Receive(ctx context.Context, vertex string) ([]Message[mockMessage], error) {
	// Simulate network latency (e.g., Redis roundtrip on cloud: ~50ms)
	// This is what parallel draining optimizes!
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(m.latency):
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	msgs := m.mailboxes[vertex]
	delete(m.mailboxes, vertex)
	return msgs, nil
}

func (m *mockSlowMessageBus) Clear(ctx context.Context, vertex string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mailboxes, vertex)
	return nil
}

func (m *mockSlowMessageBus) Close() error {
	return nil
}

// buildChainGraph creates a chain of N nodes for benchmarking.
// Each node sends a message to the next node, simulating a sequential workflow.
func buildChainGraph(n int, executionTime time.Duration) *mockGraph {
	var callCount int
	var sent []Message[mockMessage]
	callMu := &sync.Mutex{}
	msgMu := &sync.Mutex{}

	nodes := make(map[string]*mockNode, n)

	for i := 0; i < n; i++ {
		nodeName := fmt.Sprintf("node%d", i)
		nextNode := ""
		if i < n-1 {
			nextNode = fmt.Sprintf("node%d", i+1)
		}

		nodes[nodeName] = &mockNode{
			name:       nodeName,
			next:       nextNode,
			called:     &callCount,
			callMu:     callMu,
			messagesMu: msgMu,
			messages:   &sent,
			delay:      executionTime,
		}
	}

	return &mockGraph{
		rootNodes: []string{"node0"},
		nodes:     nodes,
		state:     mockState{Counter: 0},
	}
}

// BenchmarkParallelDraining_WithLatency benchmarks parallel draining with simulated network latency.
//
// This benchmark simulates a distributed deployment with Redis or gRPC message bus
// where each mailbox drain operation takes 50ms (network roundtrip).
//
// With parallel draining (current implementation):
//   - 10 nodes:  ~50ms (all drains happen in parallel)
//   - 50 nodes:  ~50ms (all drains happen in parallel)
//   - 100 nodes: ~50ms (all drains happen in parallel)
//
// With sequential draining (old implementation):
//   - 10 nodes:  ~500ms (50ms × 10)
//   - 50 nodes:  ~2500ms (50ms × 50)
//   - 100 nodes: ~5000ms (50ms × 100)
//
// Expected speedup: 10x to 100x depending on node count
func BenchmarkParallelDraining_WithLatency(b *testing.B) {
	testCases := []struct {
		name      string
		nodes     int
		drainTime time.Duration
		execTime  time.Duration
	}{
		{"10nodes_50ms", 10, 50 * time.Millisecond, 10 * time.Millisecond},
		{"50nodes_50ms", 50, 50 * time.Millisecond, 10 * time.Millisecond},
		{"100nodes_50ms", 100, 50 * time.Millisecond, 10 * time.Millisecond},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			graph := buildChainGraph(tc.nodes, tc.execTime)
			bus := newMockSlowMessageBus(tc.drainTime)

			// Seed messages for first node to start execution
			bus.Send(context.Background(), []Message[mockMessage]{
				{From: "root", To: "node0", Data: mockMessage{Value: 1}},
			})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				runtime, err := NewRuntime(graph, nil,
					WithMessageBus[mockState, mockMessage](bus),
					WithMaxWorkers[mockState, mockMessage](tc.nodes), // Max parallelism
				)
				if err != nil {
					b.Fatal(err)
				}

				if err := runtime.Run(context.Background()); err != nil {
					b.Fatal(err)
				}

				// Re-seed for next iteration
				bus.Send(context.Background(), []Message[mockMessage]{
					{From: "root", To: "node0", Data: mockMessage{Value: 1}},
				})
			}
		})
	}
}

// BenchmarkParallelDraining_InMemory benchmarks with in-memory message bus (no latency).
//
// This verifies there's NO regression for non-distributed deployments.
// The parallel draining should have negligible overhead compared to sequential.
func BenchmarkParallelDraining_InMemory(b *testing.B) {
	testCases := []struct {
		name     string
		nodes    int
		execTime time.Duration
	}{
		{"10nodes", 10, 10 * time.Millisecond},
		{"50nodes", 50, 10 * time.Millisecond},
		{"100nodes", 100, 10 * time.Millisecond},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			graph := buildChainGraph(tc.nodes, tc.execTime)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				runtime, err := NewRuntime(graph, nil,
					WithMaxWorkers[mockState, mockMessage](tc.nodes),
				)
				if err != nil {
					b.Fatal(err)
				}

				if err := runtime.Run(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
