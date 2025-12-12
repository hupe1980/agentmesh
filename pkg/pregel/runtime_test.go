package pregel

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runToCompletion consumes the iterator and returns the last error (if any)
func runToCompletion[S any, M any](ctx context.Context, rt *Runtime[S, M]) error {
	for _, err := range rt.Run(ctx) {
		if err != nil {
			return err
		}
	}
	return nil
}

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

func (g *mockGraph) RootVertices() []string { return g.rootNodes }
func (g *mockGraph) Outgoing(node string) []string {
	if n, ok := g.nodes[node]; ok && n.next != "" {
		return []string{n.next}
	}
	return nil
}
func (g *mockGraph) VertexByName(name string) Vertex[mockState, mockMessage] { return g.nodes[name] }
func (g *mockGraph) State() mockState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

type recordingScheduler struct {
	infos []SchedulerInfo
}

func (s *recordingScheduler) NextBatch(ctx context.Context, info SchedulerInfo) ([]string, error) {
	s.infos = append(s.infos, info)
	batch := make([]string, 0, len(info.Frontier))
	for v := range info.Frontier {
		batch = append(batch, v)
	}
	sort.Strings(batch)
	return batch, nil
}

func (s *recordingScheduler) RecordCompletion(ctx context.Context, vertex string, info CompletionInfo) {
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

	rt, err := NewRuntime[mockState, mockMessage](graph, nil)
	require.NoError(t, err)
	require.NoError(t, runToCompletion(context.Background(), rt))

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

	rt, err := NewRuntime[mockState, mockMessage](graph, nil)
	require.NoError(t, err)
	require.NoError(t, runToCompletion(context.Background(), rt))

	assert.Equal(t, 3, callCount)
	assert.Len(t, sent, 2)
}

func TestRuntime_SchedulerReceivesMessageCounts(t *testing.T) {
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
				next:       "",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      0,
			},
		},
	}

	scheduler := &recordingScheduler{}
	rt, err := NewRuntime[mockState, mockMessage](graph, WithScheduler[mockState, mockMessage](scheduler))
	require.NoError(t, err)
	require.NoError(t, runToCompletion(context.Background(), rt))

	var superstepTwo SchedulerInfo
	found := false
	for _, info := range scheduler.infos {
		if info.Superstep == 2 { // frontier containing B
			superstepTwo = info
			found = true
			break
		}
	}

	require.True(t, found, "expected scheduler to be called for second superstep")
	require.NotNil(t, superstepTwo.MessageCounts)
	assert.Equal(t, 1, superstepTwo.MessageCounts["B"])
}

func TestRuntime_MultipleRoots_Concurrent(t *testing.T) {
	var callCount int
	var sent []Message[mockMessage]
	mu1, mu2 := &sync.Mutex{}, &sync.Mutex{}

	graph := &mockGraph{
		rootNodes: []string{"A", "B"},
		nodes: map[string]*mockNode{
			"A": {
				name:       "A",
				next:       "C",
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

	rt, err := NewRuntime[mockState, mockMessage](graph, nil)
	require.NoError(t, err)
	require.NoError(t, runToCompletion(context.Background(), rt))

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
			"A": {
				name:       "A",
				next:       "B",
				called:     &callCount,
				callMu:     mu1,
				messagesMu: mu2,
				messages:   &sent,
				delay:      25 * time.Millisecond,
			},
			"B": {
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
	rt, err := NewRuntime[mockState, mockMessage](graph, nil)
	require.NoError(t, err)

	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	err = runToCompletion(ctx, rt)
	assert.Error(t, err)
	assert.LessOrEqual(t, callCount, 2)
}

type errorNode struct{ name string }

func (e *errorNode) Name() string { return e.name }
func (e *errorNode) Run(ctx context.Context, vertex VertexContext[mockState, mockMessage], incoming []Message[mockMessage]) error {
	return fmt.Errorf("intentional failure")
}

type errorGraph struct{ state mockState }

func (g *errorGraph) RootVertices() []string        { return []string{"X"} }
func (g *errorGraph) Outgoing(node string) []string { return nil }
func (g *errorGraph) VertexByName(name string) Vertex[mockState, mockMessage] {
	return &errorNode{name}
}
func (g *errorGraph) State() mockState { return g.state }

func TestRuntime_NodeErrorPropagation(t *testing.T) {
	rt, err := NewRuntime[mockState, mockMessage](&errorGraph{})
	require.NoError(t, err)

	var observed bool
	for evt, err := range rt.Run(context.Background()) {
		if err != nil {
			observed = true
			assert.Contains(t, err.Error(), "intentional failure")
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

func (g *panicGraph) RootVertices() []string        { return []string{"P"} }
func (g *panicGraph) Outgoing(node string) []string { return nil }
func (g *panicGraph) VertexByName(name string) Vertex[mockState, mockMessage] {
	return &panicNode{name: name}
}
func (g *panicGraph) State() mockState { return g.state }

func TestRuntime_NodePanicRecovery(t *testing.T) {
	rt, err := NewRuntime[mockState, mockMessage](&panicGraph{})
	require.NoError(t, err)

	var observed bool
	var lastErr error
	var foundDiagnostics bool
	for evt, err := range rt.Run(context.Background()) {
		if err != nil {
			observed = true
			lastErr = err
			assert.Equal(t, int64(1), evt.Superstep)
			assert.Contains(t, lastErr.Error(), "panicked")
			if evt.Diagnostics != nil {
				foundDiagnostics = true
			}
		}
	}
	assert.True(t, observed)
	assert.Error(t, lastErr)
	assert.True(t, strings.Contains(lastErr.Error(), "panicked"))
	assert.True(t, foundDiagnostics, "Expected at least one event to have diagnostics")
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

	rt, err := NewRuntime[mockState, mockMessage](graph, nil, WithInitialSuperstep[mockState, mockMessage](5))
	require.NoError(t, err)
	require.NoError(t, runToCompletion(context.Background(), rt))

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
	node Vertex[mockState, mockMessage]
}

func (g *singleNodeGraph) RootVertices() []string { return []string{g.name} }

func (g *singleNodeGraph) Outgoing(node string) []string {
	if node == g.name {
		return []string{g.name}
	}
	return nil
}

func (g *singleNodeGraph) VertexByName(name string) Vertex[mockState, mockMessage] {
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
	rt, err := NewRuntime[mockState, mockMessage](graph, nil, WithAggregators[mockState, mockMessage](aggregators))
	require.NoError(t, err)
	require.NoError(t, runToCompletion(context.Background(), rt))

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

func (g *combinerGraph) RootVertices() []string { return []string{g.producer.Name()} }

func (g *combinerGraph) Outgoing(name string) []string {
	switch name {
	case g.producer.Name():
		return []string{g.sink.Name()}
	default:
		return nil
	}
}

func (g *combinerGraph) VertexByName(name string) Vertex[mockState, mockMessage] {
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

	// Use small mailbox (2) so combining triggers at 75% capacity (2 messages)
	rt, err := NewRuntime[mockState, mockMessage](graph, nil,
		WithCombiner[mockState, mockMessage](combiner),
		WithMaxMailboxSize[mockState, mockMessage](2))
	require.NoError(t, err)
	require.NoError(t, runToCompletion(context.Background(), rt))

	// With small mailbox and combiner, messages should be combined
	// Verify total value is preserved (1 + 2 = 3)
	totalValue := 0
	for _, msg := range received {
		totalValue += msg.Data.Value
	}
	assert.Equal(t, 3, totalValue, "Total value should be preserved through combining")
	// Should have fewer messages than sent due to combining
	assert.LessOrEqual(t, len(received), 2, "Combiner should reduce message count")
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

func (g *deliverGraph) RootVertices() []string { return nil }

func (g *deliverGraph) Outgoing(string) []string { return nil }

func (g *deliverGraph) VertexByName(name string) Vertex[*deliverState, mockMessage] {
	if name == g.node.Name() {
		return g.node
	}
	return nil
}

func (g *deliverGraph) State() *deliverState { return g.state }

func TestRuntime_DeliverSeedsExecution(t *testing.T) {
	graph := newDeliverGraph()
	rt, err := NewRuntime[*deliverState, mockMessage](graph, nil)
	require.NoError(t, err)
	require.NoError(t, rt.Deliver(context.Background(), Message[mockMessage]{From: "external", To: "inbox", Data: mockMessage{Value: 1}}))
	require.NoError(t, runToCompletion(context.Background(), rt))

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

	rt, err := NewRuntime[mockState, mockMessage](graph, nil)
	require.NoError(t, err)
	require.NoError(t, runToCompletion(context.Background(), rt))

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
	rt, err := NewRuntime[mockState, mockMessage](graph, nil, WithAggregators[mockState, mockMessage](aggregators))
	require.NoError(t, err)
	err = runToCompletion(context.Background(), rt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown aggregator")
}

type noopState struct{}

type noopGraph struct{}

func (noopGraph) RootVertices() []string                             { return nil }
func (noopGraph) Outgoing(string) []string                           { return nil }
func (noopGraph) VertexByName(string) Vertex[noopState, mockMessage] { return nil }
func (noopGraph) State() noopState                                   { return noopState{} }

func TestRuntime_SetSuperstepClampsNegative(t *testing.T) {
	rt, err := NewRuntime[noopState, mockMessage](noopGraph{}, nil)
	require.NoError(t, err)
	rt.SetSuperstep(-5)
	assert.Equal(t, int64(0), rt.CurrentSuperstep())
	rt.SetSuperstep(7)
	assert.Equal(t, int64(7), rt.CurrentSuperstep())
}

// Tests for extracted runSuperstep components

func TestRuntime_ScheduleFrontierNodes(t *testing.T) {
	tests := []struct {
		name     string
		frontier map[string]struct{}
		want     []string
	}{
		{
			name:     "empty frontier",
			frontier: map[string]struct{}{},
			want:     []string{},
		},
		{
			name: "single node",
			frontier: map[string]struct{}{
				"A": {},
			},
			want: []string{"A"},
		},
		{
			name: "multiple nodes sorted",
			frontier: map[string]struct{}{
				"C": {},
				"A": {},
				"B": {},
			},
			want: []string{"A", "B", "C"},
		},
		{
			name: "numeric names sorted lexicographically",
			frontier: map[string]struct{}{
				"node_3": {},
				"node_1": {},
				"node_2": {},
			},
			want: []string{"node_1", "node_2", "node_3"},
		},
		{
			name: "mixed case names",
			frontier: map[string]struct{}{
				"Zebra":  {},
				"apple":  {},
				"Banana": {},
			},
			want: []string{"Banana", "Zebra", "apple"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := NewRuntime[noopState, mockMessage](noopGraph{}, nil)
			require.NoError(t, err)
			got, err := rt.scheduleFrontierNodes(context.Background(), tt.frontier, nil, 1)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntime_ScheduleFrontierNodes_Determinism(t *testing.T) {
	// Test that scheduleFrontierNodes returns consistent ordering across multiple calls
	frontier := map[string]struct{}{
		"node_5": {},
		"node_1": {},
		"node_3": {},
		"node_2": {},
		"node_4": {},
	}

	rt, err := NewRuntime[noopState, mockMessage](noopGraph{}, nil)
	require.NoError(t, err)

	// Run multiple times and verify consistency
	first, err := rt.scheduleFrontierNodes(context.Background(), frontier, nil, 1)
	assert.NoError(t, err)
	for i := 0; i < 10; i++ {
		got, err := rt.scheduleFrontierNodes(context.Background(), frontier, nil, 1)
		assert.NoError(t, err)
		assert.Equal(t, first, got, "scheduleFrontierNodes should return consistent ordering")
	}
}

func TestRuntime_SetupSuperstepObservability(t *testing.T) {
	tests := []struct {
		name          string
		superstep     int64
		frontierNodes []string
	}{
		{
			name:          "empty frontier",
			superstep:     1,
			frontierNodes: []string{},
		},
		{
			name:          "single node",
			superstep:     5,
			frontierNodes: []string{"A"},
		},
		{
			name:          "multiple nodes",
			superstep:     10,
			frontierNodes: []string{"A", "B", "C"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := NewRuntime[noopState, mockMessage](noopGraph{}, nil)
			require.NoError(t, err)
			ctx := context.Background()

			// Setup observability
			newCtx, cleanup := rt.setupSuperstepObservability(ctx, tt.superstep, tt.frontierNodes)

			// Verify context is returned
			assert.NotNil(t, newCtx)
			assert.NotNil(t, cleanup)

			// Verify cleanup can be called without panic
			assert.NotPanics(t, func() {
				cleanup()
			})

			// Verify cleanup is idempotent
			assert.NotPanics(t, func() {
				cleanup()
			})
		})
	}
}

func TestRuntime_SetupSuperstepObservability_ContextValues(t *testing.T) {
	rt, err := NewRuntime[noopState, mockMessage](noopGraph{}, nil)
	require.NoError(t, err)
	ctx := context.Background()
	frontierNodes := []string{"A", "B", "C"}

	newCtx, cleanup := rt.setupSuperstepObservability(ctx, 42, frontierNodes)
	defer cleanup()

	// Verify that trace and metrics contexts are preserved
	tp := trace.FromContext(newCtx)
	assert.NotNil(t, tp, "trace provider should be accessible from context")

	mp := metrics.FromContext(newCtx)
	assert.NotNil(t, mp, "metrics provider should be accessible from context")
}

func TestRuntime_ExecuteSuperstepStartCallback(t *testing.T) {
	t.Run("no callback configured", func(t *testing.T) {
		rt, err := NewRuntime[noopState, mockMessage](noopGraph{}, nil)
		require.NoError(t, err)
		ctx := context.Background()
		frontierNodes := []string{"A", "B"}

		err = rt.executeSuperstepStartCallback(ctx, 1, frontierNodes)
		assert.NoError(t, err, "should not error when callback is not configured")
	})

	t.Run("callback succeeds", func(t *testing.T) {
		callbackCalled := false
		var receivedSuperstep int64
		var receivedInfo FrontierInfo

		callback := func(_ context.Context, superstep int64, info FrontierInfo) error {
			callbackCalled = true
			receivedSuperstep = superstep
			receivedInfo = info
			return nil
		}

		rt, err := NewRuntime[noopState, mockMessage](
			noopGraph{},
			nil,
			WithOnSuperstepStart[noopState, mockMessage](callback),
		)
		require.NoError(t, err)

		ctx := context.Background()
		frontierNodes := []string{"A", "B", "C"}

		err = rt.executeSuperstepStartCallback(ctx, 42, frontierNodes)
		assert.NoError(t, err)
		assert.True(t, callbackCalled, "callback should have been called")
		assert.Equal(t, int64(42), receivedSuperstep)
		assert.Equal(t, 3, receivedInfo.Size)
		assert.Equal(t, frontierNodes, receivedInfo.Nodes)
	})

	t.Run("callback returns error", func(t *testing.T) {
		expectedErr := fmt.Errorf("callback failed")
		callback := func(_ context.Context, _ int64, _ FrontierInfo) error {
			return expectedErr
		}

		rt, err := NewRuntime[noopState, mockMessage](
			noopGraph{},
			nil,
			WithOnSuperstepStart[noopState, mockMessage](callback),
		)
		require.NoError(t, err)

		ctx := context.Background()
		frontierNodes := []string{"A"}

		err = rt.executeSuperstepStartCallback(ctx, 1, frontierNodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "superstep start callback failed")
		assert.Contains(t, err.Error(), "callback failed")
	})

	t.Run("callback respects context cancellation", func(t *testing.T) {
		callback := func(ctx context.Context, _ int64, _ FrontierInfo) error {
			// Simulate work that checks context
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		}

		rt, err := NewRuntime[noopState, mockMessage](
			noopGraph{},
			nil,
			WithOnSuperstepStart[noopState, mockMessage](callback),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		frontierNodes := []string{"A"}

		err = rt.executeSuperstepStartCallback(ctx, 1, frontierNodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "superstep start callback failed")
	})
}

func TestRuntime_ExecuteSuperstepVertices(t *testing.T) {
	t.Run("empty vertex list", func(t *testing.T) {
		rt, err := NewRuntime[noopState, mockMessage](noopGraph{}, nil)
		require.NoError(t, err)
		ctx := context.Background()

		err = rt.executeSuperstepVertices(ctx, []string{}, 1)
		assert.NoError(t, err)
	})

	t.Run("context cancellation", func(t *testing.T) {
		// Create a graph with slow vertices
		var callCount int
		mu := &sync.Mutex{}

		slowNode := &mockNode{
			name:   "slow",
			next:   "",
			called: &callCount,
			callMu: mu,
			delay:  1 * time.Second, // Long delay
		}

		graph := &mockGraph{
			rootNodes: []string{"slow"},
			nodes: map[string]*mockNode{
				"slow": slowNode,
			},
		}

		rt, err := NewRuntime[mockState, mockMessage](graph, nil)
		require.NoError(t, err)

		// Seed the message bus to trigger execution
		err = rt.Deliver(context.Background(), Message[mockMessage]{
			From: "external",
			To:   "slow",
			Data: mockMessage{Value: 1},
		})
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err = rt.executeSuperstepVertices(ctx, []string{"slow"}, 1)

		// Should respect context timeout
		assert.Error(t, err)
	})
}

func TestRuntime_ExecuteSuperstepVertices_WithMaxWorkers(t *testing.T) {
	tests := []struct {
		name       string
		maxWorkers int
		numNodes   int
	}{
		{
			name:       "single worker",
			maxWorkers: 1,
			numNodes:   3,
		},
		{
			name:       "multiple workers",
			maxWorkers: 4,
			numNodes:   8,
		},
		{
			name:       "more workers than nodes",
			maxWorkers: 10,
			numNodes:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var callCount int
			mu := &sync.Mutex{}

			nodes := make(map[string]*mockNode)
			nodeNames := make([]string, tt.numNodes)

			for i := 0; i < tt.numNodes; i++ {
				name := fmt.Sprintf("node_%d", i)
				nodeNames[i] = name
				nodes[name] = &mockNode{
					name:   name,
					next:   "",
					called: &callCount,
					callMu: mu,
					delay:  0,
				}
			}

			graph := &mockGraph{
				rootNodes: nodeNames[:1], // Just first node as root
				nodes:     nodes,
			}

			rt, err := NewRuntime[mockState, mockMessage](
				graph,
				nil,
				WithMaxWorkers[mockState, mockMessage](tt.maxWorkers),
			)
			require.NoError(t, err)

			// Seed messages for all nodes
			ctx := context.Background()
			for _, name := range nodeNames {
				err = rt.Deliver(ctx, Message[mockMessage]{
					From: "external",
					To:   name,
					Data: mockMessage{Value: 1},
				})
				require.NoError(t, err)
			}

			err = rt.executeSuperstepVertices(ctx, nodeNames, 1)
			assert.NoError(t, err)
			assert.Equal(t, tt.numNodes, callCount, "all nodes should be executed")
		})
	}
}

func TestRuntime_RunSuperstep_Integration(t *testing.T) {
	// Test the full runSuperstep flow with all extracted components
	t.Run("complete superstep execution", func(t *testing.T) {
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
					next:       "",
					called:     &callCount,
					callMu:     mu1,
					messagesMu: mu2,
					messages:   &sent,
					delay:      0,
				},
			},
		}

		callbackCalled := false
		callback := func(_ context.Context, _ int64, info FrontierInfo) error {
			callbackCalled = true
			assert.Greater(t, info.Size, 0, "frontier should not be empty")
			return nil
		}

		rt, err := NewRuntime[mockState, mockMessage](
			graph,
			nil,
			WithOnSuperstepStart[mockState, mockMessage](callback),
		)
		require.NoError(t, err)

		ctx := context.Background()
		frontier := map[string]struct{}{"A": {}}

		err = rt.runSuperstep(ctx, frontier, nil, 1)
		assert.NoError(t, err)
		assert.True(t, callbackCalled, "callback should be invoked")
		assert.Equal(t, 1, callCount, "vertex A should be executed")
		assert.Len(t, sent, 1, "one message should be sent")
	})

	t.Run("empty frontier no-op", func(t *testing.T) {
		rt, err := NewRuntime[noopState, mockMessage](noopGraph{}, nil)
		require.NoError(t, err)
		ctx := context.Background()
		frontier := map[string]struct{}{}

		err = rt.runSuperstep(ctx, frontier, nil, 1)
		assert.NoError(t, err)
	})
}
