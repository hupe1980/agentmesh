// Package testutil provides common mock implementations for testing across the agentmesh codebase.
// This centralizes mock patterns to reduce code duplication and improve test maintainability.
package testutil

import (
	"context"
	"errors"
	"iter"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/pregel"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// MockModel is a configurable mock implementation of model.Model.
// Use GenerateFunc to customize response generation behavior.
type MockModel struct {
	GenerateFunc     func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error]
	CapabilitiesFunc func() model.Capabilities
}

// Generate returns a sequence of model responses.
// If GenerateFunc is set, it delegates to that function.
// Otherwise, returns a default single AI message response.
func (m *MockModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(ctx, req)
	}
	// Default implementation returns a single message
	return func(yield func(*model.Response, error) bool) {
		yield(&model.Response{
			Message: message.NewAIMessageFromText("mock response"),
			Partial: false, // Single complete response
		}, nil)
	}
}

// Capabilities returns the model's capabilities.
// If CapabilitiesFunc is set, it delegates to that function.
// Otherwise, returns default capabilities with streaming and tools enabled.
func (m *MockModel) Capabilities() model.Capabilities {
	if m.CapabilitiesFunc != nil {
		return m.CapabilitiesFunc()
	}
	// Default capabilities
	return model.Capabilities{
		Streaming:           true,
		Tools:               true,
		MaxContextTokens:    4096,
		MaxOutputTokens:     2048,
		SupportedModalities: []string{"text"},
	}
}

// WrapSimpleGenerate wraps a simple generate function into an iterator for MockModel.GenerateFunc.
// This helper makes it easy to create simple mocks that don't need streaming.
func WrapSimpleGenerate(fn func(ctx context.Context, messages []message.Message) (message.Message, error)) func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			msg, err := fn(ctx, req.Messages)
			yield(&model.Response{Message: msg, Partial: false}, err)
		}
	}
}

// MockTool is a configurable mock implementation of tool.Tool.
type MockTool struct {
	NameValue        string
	DescriptionValue string
	CallFunc         func(ctx context.Context, args string) (any, error)
	SchemaValue      *tool.Definition
}

// Name returns the tool's name.
func (t *MockTool) Name() string {
	if t.NameValue == "" {
		return "mock_tool"
	}
	return t.NameValue
}

// Description returns the tool's description.
func (t *MockTool) Description() string {
	if t.DescriptionValue == "" {
		return "A mock tool for testing"
	}
	return t.DescriptionValue
}

// Definition returns the tool's schema definition.
func (t *MockTool) Definition() *tool.Definition {
	if t.SchemaValue != nil {
		return t.SchemaValue
	}
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{
						"type":        "string",
						"description": "Input parameter",
					},
				},
			},
		},
	}
}

// Call executes the tool with the given arguments.
// If CallFunc is set, it delegates to that function.
// Otherwise, returns a default "mock result" string.
func (t *MockTool) Call(ctx context.Context, args string) (any, error) {
	if t.CallFunc != nil {
		return t.CallFunc(ctx, args)
	}
	return "mock result", nil
}

// MockNode is a mock implementation of pregel.Node for testing BSP graphs.
type MockNode[S any, M any] struct {
	NameValue  string
	NextNode   string
	Called     *int
	CallMu     *sync.Mutex
	MessagesMu *sync.Mutex
	Messages   *[]pregel.Message[M]
	Delay      *func() // Optional delay function for simulating work
	RunFunc    func(ctx context.Context, vertex pregel.VertexContext[S, M], incoming []pregel.Message[M]) error
}

// Name returns the node's identifier.
func (n *MockNode[S, M]) Name() string {
	if n.NameValue == "" {
		return "mock_node"
	}
	return n.NameValue
}

// Run executes the node's computation.
// If RunFunc is set, it delegates to that function.
// Otherwise, implements default behavior: increments counter, optionally sends message to NextNode.
func (n *MockNode[S, M]) Run(ctx context.Context, vertex pregel.VertexContext[S, M], incoming []pregel.Message[M]) error {
	if n.RunFunc != nil {
		return n.RunFunc(ctx, vertex, incoming)
	}

	// Default implementation: track calls and propagate messages
	if n.CallMu != nil && n.Called != nil {
		n.CallMu.Lock()
		*n.Called++
		n.CallMu.Unlock()
	}

	// Consume incoming messages (default: no-op)
	_ = incoming

	// Optional delay to simulate work
	if n.Delay != nil && *n.Delay != nil {
		(*n.Delay)()
	}

	return nil
}

// MockGraph is a mock implementation of pregel.Graph for testing.
type MockGraph[S any, M any] struct {
	RootNodesValue []string
	Nodes          map[string]pregel.Node[S, M]
	OutgoingEdges  map[string][]string
	StateValue     S
	StateMu        sync.Mutex
}

// RootNodes returns the initial vertices to activate.
func (g *MockGraph[S, M]) RootNodes() []string {
	return g.RootNodesValue
}

// Outgoing returns the destination vertices for a given node.
func (g *MockGraph[S, M]) Outgoing(node string) []string {
	if edges, ok := g.OutgoingEdges[node]; ok {
		return edges
	}
	return nil
}

// NodeByName returns the node with the given name.
func (g *MockGraph[S, M]) NodeByName(name string) pregel.Node[S, M] {
	return g.Nodes[name]
}

// State returns the current graph state.
func (g *MockGraph[S, M]) State() S {
	g.StateMu.Lock()
	defer g.StateMu.Unlock()
	return g.StateValue
}

// NewMockGraph creates a new MockGraph with the given configuration.
func NewMockGraph[S any, M any](rootNodes []string, nodes map[string]pregel.Node[S, M], edges map[string][]string, state S) *MockGraph[S, M] {
	return &MockGraph[S, M]{
		RootNodesValue: rootNodes,
		Nodes:          nodes,
		OutgoingEdges:  edges,
		StateValue:     state,
	}
}

// MockCheckpointer is a mock implementation of checkpoint.Checkpointer for testing.
// It stores checkpoints in memory with configurable behavior.
type MockCheckpointer struct {
	SaveFunc            func(ctx context.Context, cp *checkpoint.Checkpoint) error
	LoadFunc            func(ctx context.Context, runID string) (*checkpoint.Checkpoint, error)
	ListFunc            func(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error)
	DeleteFunc          func(ctx context.Context, runID string) error
	LoadAtSuperstepFunc func(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error)

	// Storage holds checkpoints by runID
	Storage map[string][]*checkpoint.Checkpoint
	mu      sync.RWMutex
}

// NewMockCheckpointer creates a new MockCheckpointer with in-memory storage.
func NewMockCheckpointer() *MockCheckpointer {
	return &MockCheckpointer{
		Storage: make(map[string][]*checkpoint.Checkpoint),
	}
}

// Save persists a checkpoint.
// If SaveFunc is set, delegates to it. Otherwise uses default in-memory storage.
func (m *MockCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, cp)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.Storage[cp.RunID] = append(m.Storage[cp.RunID], cp)
	return nil
}

// Load retrieves the most recent checkpoint for a run ID.
// If LoadFunc is set, delegates to it. Otherwise uses default in-memory storage.
func (m *MockCheckpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	if m.LoadFunc != nil {
		return m.LoadFunc(ctx, runID)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints := m.Storage[runID]
	if len(checkpoints) == 0 {
		return nil, nil // No checkpoint exists
	}

	// Return most recent checkpoint
	return checkpoints[len(checkpoints)-1], nil
}

// List returns all checkpoints for a run ID, ordered by superstep (newest first).
// If ListFunc is set, delegates to it. Otherwise uses default in-memory storage.
func (m *MockCheckpointer) List(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, runID)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints := m.Storage[runID]
	if len(checkpoints) == 0 {
		return []*checkpoint.Checkpoint{}, nil
	}

	// Return copy in reverse order (newest first)
	result := make([]*checkpoint.Checkpoint, len(checkpoints))
	for i, cp := range checkpoints {
		result[len(checkpoints)-1-i] = cp
	}

	return result, nil
}

// Delete removes all checkpoints for a run ID.
// If DeleteFunc is set, delegates to it. Otherwise uses default in-memory storage.
func (m *MockCheckpointer) Delete(ctx context.Context, runID string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, runID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.Storage[runID]; !exists {
		return errors.New("no checkpoints found")
	}

	delete(m.Storage, runID)
	return nil
}

// LoadAtSuperstep retrieves a checkpoint at a specific superstep.
// If LoadAtSuperstepFunc is set, delegates to it. Otherwise uses default in-memory storage.
func (m *MockCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	if m.LoadAtSuperstepFunc != nil {
		return m.LoadAtSuperstepFunc(ctx, runID, superstep)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints := m.Storage[runID]
	for _, cp := range checkpoints {
		if cp.Superstep == superstep {
			return cp, nil
		}
	}

	return nil, nil // No checkpoint at this superstep
}

// MockMetricsProvider is a mock implementation of metrics.Provider for testing.
// It tracks all metric operations for verification in tests.
type MockMetricsProvider struct {
	mu                 sync.Mutex
	counterCalls       int
	histogramCalls     int
	lastCounterName    string
	lastCounterValue   float64
	lastCounterAttrs   []any // Using any to avoid import cycle
	lastHistogramName  string
	lastHistogramValue float64
	lastHistogramAttrs []any

	// Counters tracks all counter add operations
	Counters map[string]float64
	// Histograms tracks all histogram record operations
	Histograms map[string][]float64
}

// NewMockMetricsProvider creates a new MockMetricsProvider.
func NewMockMetricsProvider() *MockMetricsProvider {
	return &MockMetricsProvider{
		Counters:   make(map[string]float64),
		Histograms: make(map[string][]float64),
	}
}

// Counter returns a mock counter that tracks operations.
func (m *MockMetricsProvider) Counter(name string) interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counterCalls++
	m.lastCounterName = name
	return &mockCounter{provider: m, name: name}
}

// Histogram returns a mock histogram that tracks operations.
func (m *MockMetricsProvider) Histogram(name string) interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.histogramCalls++
	m.lastHistogramName = name
	return &mockHistogram{provider: m, name: name}
}

// GetCounterCalls returns the number of times Counter() was called.
func (m *MockMetricsProvider) GetCounterCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counterCalls
}

// GetHistogramCalls returns the number of times Histogram() was called.
func (m *MockMetricsProvider) GetHistogramCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.histogramCalls
}

// GetCounterValue returns the total value added to a specific counter.
func (m *MockMetricsProvider) GetCounterValue(name string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Counters[name]
}

// GetHistogramValues returns all values recorded to a specific histogram.
func (m *MockMetricsProvider) GetHistogramValues(name string) []float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	values := m.Histograms[name]
	result := make([]float64, len(values))
	copy(result, values)
	return result
}

type mockCounter struct {
	provider *MockMetricsProvider
	name     string
}

func (c *mockCounter) Add(ctx context.Context, value float64, attrs ...any) {
	c.provider.mu.Lock()
	defer c.provider.mu.Unlock()
	c.provider.lastCounterValue = value
	c.provider.lastCounterAttrs = attrs
	c.provider.Counters[c.name] += value
}

type mockHistogram struct {
	provider *MockMetricsProvider
	name     string
}

func (h *mockHistogram) Record(ctx context.Context, value float64, attrs ...any) {
	h.provider.mu.Lock()
	defer h.provider.mu.Unlock()
	h.provider.lastHistogramValue = value
	h.provider.lastHistogramAttrs = attrs
	h.provider.Histograms[h.name] = append(h.provider.Histograms[h.name], value)
}

// MockTraceProvider is a mock implementation of trace.Provider for testing.
// It tracks all tracing operations for verification in tests.
type MockTraceProvider struct {
	mu             sync.Mutex
	tracerCalls    int
	startCalls     int
	endCalls       int
	lastTracerName string
	lastSpanName   string
	lastStartAttrs []any
	lastEndError   error

	// Spans tracks all created spans by name
	Spans map[string]*MockSpan
}

// NewMockTraceProvider creates a new MockTraceProvider.
func NewMockTraceProvider() *MockTraceProvider {
	return &MockTraceProvider{
		Spans: make(map[string]*MockSpan),
	}
}

// Tracer returns a mock tracer that tracks operations.
func (m *MockTraceProvider) Tracer(name string) interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tracerCalls++
	m.lastTracerName = name
	return &mockTracer{provider: m}
}

// GetTracerCalls returns the number of times Tracer() was called.
func (m *MockTraceProvider) GetTracerCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tracerCalls
}

// GetStartCalls returns the number of times Start() was called.
func (m *MockTraceProvider) GetStartCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startCalls
}

// GetEndCalls returns the number of times End() was called.
func (m *MockTraceProvider) GetEndCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.endCalls
}

// GetLastSpanName returns the name of the most recently started span.
func (m *MockTraceProvider) GetLastSpanName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastSpanName
}

// GetSpan returns a mock span by name.
func (m *MockTraceProvider) GetSpan(name string) *MockSpan {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Spans[name]
}

type mockTracer struct {
	provider *MockTraceProvider
}

func (t *mockTracer) Start(ctx context.Context, name string, attrs ...any) (context.Context, interface{}) {
	t.provider.mu.Lock()
	defer t.provider.mu.Unlock()

	t.provider.startCalls++
	t.provider.lastSpanName = name
	t.provider.lastStartAttrs = attrs

	span := &MockSpan{
		provider: t.provider,
		Name:     name,
	}
	t.provider.Spans[name] = span

	type spanKey struct{}
	return context.WithValue(ctx, spanKey{}, span), span
}

// MockSpan is a mock implementation of trace.Span for testing.
type MockSpan struct {
	provider *MockTraceProvider
	Name     string
	Ended    bool
	Error    error
}

// End marks the span as complete.
func (s *MockSpan) End(err error) {
	s.provider.mu.Lock()
	defer s.provider.mu.Unlock()

	s.provider.endCalls++
	s.provider.lastEndError = err
	s.Ended = true
	s.Error = err
}
