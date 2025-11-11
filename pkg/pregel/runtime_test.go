package pregel

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockState struct{ Counter int }
type mockMessage struct{ Value int }

type mockNode struct {
	name       string
	next       string
	called     *int
	callMu     *sync.Mutex
	messagesMu *sync.Mutex
	messages   *[]Message[mockMessage]
	delay      time.Duration
}

func (n *mockNode) Name() string { return n.name }
func (n *mockNode) Run(ctx context.Context, vertex VertexContext[mockState, mockMessage], incoming []Message[mockMessage]) error {
	n.callMu.Lock()
	*n.called++
	n.callMu.Unlock()

	for range incoming {
		// optional: consume messages
	}

	if n.delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(n.delay):
		}
	}

	if n.next != "" {
		msg := Message[mockMessage]{From: n.name, To: n.next, Data: mockMessage{Value: vertex.State.Counter + 1}}
		vertex.Send(msg)

		n.messagesMu.Lock()
		*n.messages = append(*n.messages, msg)
		n.messagesMu.Unlock()
	}

	return nil
}

type mockGraph struct {
	rootNodes []string
	nodes     map[string]*mockNode
	state     mockState
	mu        sync.Mutex
}

func (g *mockGraph) RootNodes() []string { return g.rootNodes }
func (g *mockGraph) Outgoing(node string) []string {
	if n, ok := g.nodes[node]; ok && n.next != "" {
		return []string{n.next}
	}
	return nil
}
func (g *mockGraph) NodeByName(name string) Node[mockState, mockMessage] { return g.nodes[name] }
func (g *mockGraph) State() mockState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

func TestRuntime_Run_SequentialGraph(t *testing.T) {
	var callCount int
	var sent []Message[mockMessage]
	mu1, mu2 := &sync.Mutex{}, &sync.Mutex{}

	graph := &mockGraph{
		rootNodes: []string{"A"},
		nodes: map[string]*mockNode{
			"A": {
				name:       "A",
				next:       "B",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
			"B": {
				name:       "B",
				next:       "C",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
			"C": {
				name:       "C",
				next:       "",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
		},
	}

	rt := MustNewRuntime[mockState, mockMessage](graph, nil)
	require.NoError(t, rt.Run(context.Background()))

	assert.Equal(t, 3, callCount)
	assert.Len(t, sent, 2)
	assert.Equal(t, "A", sent[0].From)
	assert.Equal(t, "B", sent[1].From)
}

func TestRuntime_MessagePropagation(t *testing.T) {
	var callCount int
	var sent []Message[mockMessage]
	mu1, mu2 := &sync.Mutex{}, &sync.Mutex{}

	graph := &mockGraph{
		rootNodes: []string{"A"},
		nodes: map[string]*mockNode{
			"A": &mockNode{
				name:       "A",
				next:       "B",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
			"B": &mockNode{
				name:       "B",
				next:       "C",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
			"C": &mockNode{
				name:       "C",
				next:       "",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
		},
	}

	rt := MustNewRuntime[mockState, mockMessage](graph, nil)
	require.NoError(t, rt.Run(context.Background()))

	assert.Equal(t, 3, callCount)
	assert.Len(t, sent, 2)
}

func TestRuntime_MultipleRoots_Concurrent(t *testing.T) {
	var callCount int
	var sent []Message[mockMessage]
	mu1, mu2 := &sync.Mutex{}, &sync.Mutex{}

	graph := &mockGraph{
		rootNodes: []string{"A", "B"},
		nodes: map[string]*mockNode{
			"A": &mockNode{
				name:       "A",
				next:       "C",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
			"B": &mockNode{
				name:       "B",
				next:       "C",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
			"C": &mockNode{
				name:       "C",
				next:       "",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
		},
	}

	rt := MustNewRuntime[mockState, mockMessage](graph, nil)
	require.NoError(t, rt.Run(context.Background()))

	assert.Equal(t, 3, callCount)
	assert.Len(t, sent, 2)
}

func TestRuntime_CancelDuringExecution(t *testing.T) {
	var callCount int
	var sent []Message[mockMessage]
	mu1, mu2 := &sync.Mutex{}, &sync.Mutex{}

	graph := &mockGraph{
		rootNodes: []string{"A", "B"},
		nodes: map[string]*mockNode{
			"A": &mockNode{
				name:       "A",
				next:       "B",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      25 * time.Millisecond,
			},
			"B": &mockNode{
				name:       "B",
				next:       "",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      25 * time.Millisecond,
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	rt := MustNewRuntime[mockState, mockMessage](graph, nil)

	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	err := rt.Run(ctx)
	assert.Error(t, err)
	assert.LessOrEqual(t, callCount, 2)
}

type errorNode struct{ name string }

func (e *errorNode) Name() string { return e.name }
func (e *errorNode) Run(ctx context.Context, vertex VertexContext[mockState, mockMessage], incoming []Message[mockMessage]) error {
	return fmt.Errorf("intentional failure")
}

type errorGraph struct{ state mockState }

func (g *errorGraph) RootNodes() []string           { return []string{"X"} }
func (g *errorGraph) Outgoing(node string) []string { return nil }
func (g *errorGraph) NodeByName(name string) Node[mockState, mockMessage] {
	return &errorNode{name}
}
func (g *errorGraph) State() mockState { return g.state }

func TestRuntime_NodeErrorPropagation(t *testing.T) {
	events := make(chan Event[mockMessage], 1)
	rt := MustNewRuntime[mockState, mockMessage](&errorGraph{}, events)

	err := rt.Run(context.Background())
	assert.Error(t, err)

	close(events)
	var observed bool
	for evt := range events {
		if evt.Error != nil {
			observed = true
			assert.Contains(t, evt.Error.Error(), "intentional failure")
			assert.Equal(t, int64(1), evt.Superstep)
		}
	}
	assert.True(t, observed)
}

type panicNode struct{ name string }

func (n *panicNode) Name() string { return n.name }
func (n *panicNode) Run(ctx context.Context, vertex VertexContext[mockState, mockMessage], incoming []Message[mockMessage]) error {
	panic("boom")
}

type panicGraph struct{ state mockState }

func (g *panicGraph) RootNodes() []string           { return []string{"P"} }
func (g *panicGraph) Outgoing(node string) []string { return nil }
func (g *panicGraph) NodeByName(name string) Node[mockState, mockMessage] {
	return &panicNode{name: name}
}
func (g *panicGraph) State() mockState { return g.state }

func TestRuntime_NodePanicRecovery(t *testing.T) {
	events := make(chan Event[mockMessage], 1)
	rt := MustNewRuntime[mockState, mockMessage](&panicGraph{}, events)

	err := rt.Run(context.Background())
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "panicked"))

	close(events)
	var observed bool
	for evt := range events {
		if evt.Error != nil {
			observed = true
			assert.Equal(t, int64(1), evt.Superstep)
			assert.Contains(t, evt.Error.Error(), "panicked")
			assert.NotNil(t, evt.Diagnostics)
		}
	}
	assert.True(t, observed)
}

func TestRuntime_InitialSuperstep(t *testing.T) {
	var callCount int
	var sent []Message[mockMessage]
	mu1, mu2 := &sync.Mutex{}, &sync.Mutex{}

	graph := &mockGraph{
		rootNodes: []string{"A"},
		nodes: map[string]*mockNode{
			"A": {
				name:       "A",
				next:       "",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
			},
		},
	}

	rt := MustNewRuntime[mockState, mockMessage](graph, nil, WithInitialSuperstep[mockState, mockMessage](5))
	require.NoError(t, rt.Run(context.Background()))

	stats := rt.Stats()
	assert.Equal(t, int64(6), stats.Supersteps)
}

type sumAggregator struct{}

func (sumAggregator) Zero() any { return 0 }

func (sumAggregator) Aggregate(current, value any) any {
	cur, _ := current.(int)
	add, _ := value.(int)
	return cur + add
}

type aggregatorProbe struct {
	observed *[]int
	sent     bool
}

func (n *aggregatorProbe) Name() string { return "agg" }

func (n *aggregatorProbe) Run(_ context.Context, vertex VertexContext[mockState, mockMessage], _ []Message[mockMessage]) error {
	val := 0
	if vertex.Aggregates != nil {
		if v, ok := vertex.Aggregates["sum"].(int); ok {
			val = v
		}
	}
	*n.observed = append(*n.observed, val)
	if err := vertex.Aggregate("sum", 1); err != nil {
		return err
	}
	if !n.sent {
		n.sent = true
		vertex.Send(Message[mockMessage]{From: "agg", To: "agg", Data: mockMessage{}})
	}
	return nil
}

type singleNodeGraph struct {
	name string
	node Node[mockState, mockMessage]
}

func (g *singleNodeGraph) RootNodes() []string { return []string{g.name} }

func (g *singleNodeGraph) Outgoing(node string) []string {
	if node == g.name {
		return []string{g.name}
	}
	return nil
}

func (g *singleNodeGraph) NodeByName(name string) Node[mockState, mockMessage] {
	if name == g.name {
		return g.node
	}
	return nil
}

func (g *singleNodeGraph) State() mockState { return mockState{} }

func TestRuntime_AggregatorsVisibleNextSuperstep(t *testing.T) {
	var observed []int
	node := &aggregatorProbe{observed: &observed}
	graph := &singleNodeGraph{name: "agg", node: node}

	aggregators := map[string]Aggregator{"sum": sumAggregator{}}
	rt := MustNewRuntime[mockState, mockMessage](graph, nil, WithAggregators[mockState, mockMessage](aggregators))
	require.NoError(t, rt.Run(context.Background()))

	require.Len(t, observed, 2)
	assert.Equal(t, []int{0, 1}, observed)
}

type combinerProducer struct {
	sent bool
}

func (n *combinerProducer) Name() string { return "producer" }

func (n *combinerProducer) Run(_ context.Context, vertex VertexContext[mockState, mockMessage], _ []Message[mockMessage]) error {
	if n.sent {
		return nil
	}
	n.sent = true
	vertex.Send(Message[mockMessage]{From: "producer", To: "sink", Data: mockMessage{Value: 1}})
	vertex.Send(Message[mockMessage]{From: "producer", To: "sink", Data: mockMessage{Value: 2}})
	return nil
}

type combinerSink struct {
	received *[]Message[mockMessage]
}

func (n *combinerSink) Name() string { return "sink" }

func (n *combinerSink) Run(_ context.Context, _ VertexContext[mockState, mockMessage], incoming []Message[mockMessage]) error {
	*n.received = append(*n.received, incoming...)
	return nil
}

type combinerGraph struct {
	producer *combinerProducer
	sink     *combinerSink
}

func (g *combinerGraph) RootNodes() []string { return []string{g.producer.Name()} }

func (g *combinerGraph) Outgoing(name string) []string {
	switch name {
	case g.producer.Name():
		return []string{g.sink.Name()}
	default:
		return nil
	}
}

func (g *combinerGraph) NodeByName(name string) Node[mockState, mockMessage] {
	switch name {
	case g.producer.Name():
		return g.producer
	case g.sink.Name():
		return g.sink
	default:
		return nil
	}
}

func (g *combinerGraph) State() mockState { return mockState{} }

func TestRuntime_CombinerMergesMessages(t *testing.T) {
	var received []Message[mockMessage]
	graph := &combinerGraph{
		producer: &combinerProducer{},
		sink:     &combinerSink{received: &received},
	}

	combiner := func(existing, incoming Message[mockMessage]) Message[mockMessage] {
		combined := existing
		combined.Data.Value += incoming.Data.Value
		return combined
	}

	rt := MustNewRuntime[mockState, mockMessage](graph, nil, WithCombiner[mockState, mockMessage](combiner))
	require.NoError(t, rt.Run(context.Background()))

	require.Len(t, received, 1)
	assert.Equal(t, 3, received[0].Data.Value)
}

type deliverState struct {
	mu    sync.Mutex
	count int
}

type deliverNode struct {
	state *deliverState
}

func (n *deliverNode) Name() string { return "inbox" }

func (n *deliverNode) Run(_ context.Context, vertex VertexContext[*deliverState, mockMessage], incoming []Message[mockMessage]) error {
	if len(incoming) == 0 {
		return nil
	}
	vertex.State.mu.Lock()
	vertex.State.count += len(incoming)
	vertex.State.mu.Unlock()
	return nil
}

type deliverGraph struct {
	state *deliverState
	node  *deliverNode
}

func newDeliverGraph() *deliverGraph {
	state := &deliverState{}
	node := &deliverNode{state: state}
	return &deliverGraph{state: state, node: node}
}

func (g *deliverGraph) RootNodes() []string { return nil }

func (g *deliverGraph) Outgoing(string) []string { return nil }

func (g *deliverGraph) NodeByName(name string) Node[*deliverState, mockMessage] {
	if name == g.node.Name() {
		return g.node
	}
	return nil
}

func (g *deliverGraph) State() *deliverState { return g.state }

func TestRuntime_DeliverSeedsExecution(t *testing.T) {
	graph := newDeliverGraph()
	rt := MustNewRuntime[*deliverState, mockMessage](graph, nil)
	require.NoError(t, rt.Deliver(context.Background(), Message[mockMessage]{From: "external", To: "inbox", Data: mockMessage{Value: 1}}))
	require.NoError(t, rt.Run(context.Background()))

	graph.state.mu.Lock()
	count := graph.state.count
	graph.state.mu.Unlock()
	assert.GreaterOrEqual(t, count, 1)

	stats := rt.Stats()
	assert.GreaterOrEqual(t, stats.Supersteps, int64(1))
	assert.GreaterOrEqual(t, stats.Vertices, int64(1))
	assert.Equal(t, int64(0), stats.Messages)
}

func TestRuntime_StatsReflectExecution(t *testing.T) {
	var callCount int
	var sent []Message[mockMessage]
	mu1, mu2 := &sync.Mutex{}, &sync.Mutex{}

	graph := &mockGraph{
		rootNodes: []string{"A"},
		nodes: map[string]*mockNode{
			"A": {
				name:       "A",
				next:       "B",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
			},
			"B": {
				name:       "B",
				next:       "C",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
			},
			"C": {
				name:       "C",
				next:       "",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
			},
		},
	}

	rt := MustNewRuntime[mockState, mockMessage](graph, nil)
	require.NoError(t, rt.Run(context.Background()))

	stats := rt.Stats()
	assert.Equal(t, int64(3), stats.Supersteps)
	assert.Equal(t, int64(3), stats.Vertices)
	assert.Equal(t, int64(2), stats.Messages)
}

type aggregatorErrorNode struct{}

func (aggregatorErrorNode) Name() string { return "agg" }

func (aggregatorErrorNode) Run(_ context.Context, vertex VertexContext[mockState, mockMessage], _ []Message[mockMessage]) error {
	return vertex.Aggregate("missing", 1)
}

func TestRuntime_AggregatorUnknownName(t *testing.T) {
	graph := &singleNodeGraph{name: "agg", node: aggregatorErrorNode{}}
	aggregators := map[string]Aggregator{"sum": sumAggregator{}}
	rt := MustNewRuntime[mockState, mockMessage](graph, nil, WithAggregators[mockState, mockMessage](aggregators))
	err := rt.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown aggregator")
}

type noopState struct{}

type noopGraph struct{}

func (noopGraph) RootNodes() []string                                  { return nil }
func (noopGraph) Outgoing(string) []string                             { return nil }
func (noopGraph) NodeByName(string) Node[noopState, mockMessage] { return nil }
func (noopGraph) State() noopState                                     { return noopState{} }

func TestRuntime_SetSuperstepClampsNegative(t *testing.T) {
	rt := MustNewRuntime[noopState, mockMessage](noopGraph{}, nil)
	rt.SetSuperstep(-5)
	assert.Equal(t, int64(0), rt.CurrentSuperstep())
	rt.SetSuperstep(7)
	assert.Equal(t, int64(7), rt.CurrentSuperstep())
}
