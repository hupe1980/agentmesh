// Package testutil provides common mock implementations for testing across the agentmesh codebase.
// This centralizes mock patterns to reduce code duplication and improve test maintainability.
package testutil

import (
	"context"
	"errors"
	"iter"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/memory"
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

// MockNode is a mock implementation of pregel.Vertex for testing BSP graphs.
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

// Name returns the vertex's identifier.
func (n *MockNode[S, M]) Name() string {
	if n.NameValue == "" {
		return "mock_node"
	}
	return n.NameValue
}

// Run executes the vertex's computation.
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
	Nodes          map[string]pregel.Vertex[S, M]
	OutgoingEdges  map[string][]string
	StateValue     S
	StateMu        sync.Mutex
}

// RootVertices returns the initial vertices to activate.
func (g *MockGraph[S, M]) RootVertices() []string {
	return g.RootNodesValue
}

// Outgoing returns the destination vertices for a given vertex.
// Returns nil if the vertex has no outgoing edges.
func (g *MockGraph[S, M]) Outgoing(vertex string) []string {
	return g.OutgoingEdges[vertex]
}

// VertexByName returns the vertex with the given name.
func (g *MockGraph[S, M]) VertexByName(name string) pregel.Vertex[S, M] {
	return g.Nodes[name]
}

// State returns the current graph state.
func (g *MockGraph[S, M]) State() S {
	g.StateMu.Lock()
	defer g.StateMu.Unlock()
	return g.StateValue
}

// NewMockGraph creates a new MockGraph with the given configuration.
func NewMockGraph[S any, M any](rootNodes []string, nodes map[string]pregel.Vertex[S, M], edges map[string][]string, state S) *MockGraph[S, M] {
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
	SaveFunc                 func(ctx context.Context, cp *checkpoint.Checkpoint) error
	LoadFunc                 func(ctx context.Context, runID string) (*checkpoint.Checkpoint, error)
	ListFunc                 func(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error)
	DeleteFunc               func(ctx context.Context, runID string) error
	LoadAtSuperstepFunc      func(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error)
	ListPendingApprovalsFunc func(ctx context.Context) ([]*checkpoint.Checkpoint, error)
	GetApprovalHistoryFunc   func(ctx context.Context, runID string) ([]checkpoint.ApprovalRecord, error)

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

// ListPendingApprovals returns all checkpoints with pending approvals.
// If ListPendingApprovalsFunc is set, delegates to it. Otherwise scans all checkpoints.
func (m *MockCheckpointer) ListPendingApprovals(ctx context.Context) ([]*checkpoint.Checkpoint, error) {
	if m.ListPendingApprovalsFunc != nil {
		return m.ListPendingApprovalsFunc(ctx)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*checkpoint.Checkpoint
	for _, checkpoints := range m.Storage {
		if len(checkpoints) > 0 {
			// Check the most recent checkpoint for each run
			latest := checkpoints[len(checkpoints)-1]
			if latest.ApprovalMetadata != nil && len(latest.ApprovalMetadata.PendingApprovals) > 0 {
				result = append(result, latest)
			}
		}
	}

	return result, nil
}

// GetApprovalHistory retrieves the approval history for a run ID.
// If GetApprovalHistoryFunc is set, delegates to it. Otherwise aggregates from all checkpoints.
func (m *MockCheckpointer) GetApprovalHistory(ctx context.Context, runID string) ([]checkpoint.ApprovalRecord, error) {
	if m.GetApprovalHistoryFunc != nil {
		return m.GetApprovalHistoryFunc(ctx, runID)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints := m.Storage[runID]
	if len(checkpoints) == 0 {
		return []checkpoint.ApprovalRecord{}, nil
	}

	// Get the most recent checkpoint with approval metadata
	for i := len(checkpoints) - 1; i >= 0; i-- {
		cp := checkpoints[i]
		if cp.ApprovalMetadata != nil && len(cp.ApprovalMetadata.ApprovalHistory) > 0 {
			return cp.ApprovalMetadata.ApprovalHistory, nil
		}
	}

	return []checkpoint.ApprovalRecord{}, nil
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

// MockMemory is a configurable mock implementation of memory.Memory.
// It stores messages in-memory with optional custom behavior.
type MockMemory struct {
	mu       sync.Mutex
	sessions map[string][]message.Message

	StoreFunc    func(ctx context.Context, sessionID string, messages []message.Message) error
	RecallFunc   func(ctx context.Context, sessionID string, filter memory.RecallFilter) ([]message.Message, error)
	ClearFunc    func(ctx context.Context, sessionID string) error
	SessionsFunc func(ctx context.Context) ([]string, error)
}

// NewMockMemory creates a new MockMemory with in-memory storage.
func NewMockMemory() *MockMemory {
	return &MockMemory{
		sessions: make(map[string][]message.Message),
	}
}

// Store persists messages for a given session.
// If StoreFunc is set, delegates to it. Otherwise uses default in-memory storage.
func (m *MockMemory) Store(ctx context.Context, sessionID string, messages []message.Message) error {
	if m.StoreFunc != nil {
		return m.StoreFunc(ctx, sessionID, messages)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sessions == nil {
		m.sessions = make(map[string][]message.Message)
	}

	m.sessions[sessionID] = append(m.sessions[sessionID], messages...)
	return nil
}

// Recall retrieves messages for a session.
// If RecallFunc is set, delegates to it. Otherwise returns stored messages.
func (m *MockMemory) Recall(ctx context.Context, sessionID string, filter memory.RecallFilter) ([]message.Message, error) {
	if m.RecallFunc != nil {
		return m.RecallFunc(ctx, sessionID, filter)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	msgs, ok := m.sessions[sessionID]
	if !ok {
		return []message.Message{}, nil
	}

	// Return up to k messages
	k := filter.K
	if k <= 0 {
		k = 10
	}
	if k < len(msgs) {
		return msgs[len(msgs)-k:], nil
	}
	return msgs, nil
}

// Clear removes all messages for a session.
// If ClearFunc is set, delegates to it. Otherwise clears in-memory storage.
func (m *MockMemory) Clear(ctx context.Context, sessionID string) error {
	if m.ClearFunc != nil {
		return m.ClearFunc(ctx, sessionID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)
	return nil
}

// Sessions returns all session IDs.
// If SessionsFunc is set, delegates to it. Otherwise returns stored session IDs.
func (m *MockMemory) Sessions(ctx context.Context) ([]string, error) {
	if m.SessionsFunc != nil {
		return m.SessionsFunc(ctx)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids, nil
}

// GetStoredMessages returns all stored messages for a session (for test assertions).
func (m *MockMemory) GetStoredMessages(sessionID string) []message.Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msgs, ok := m.sessions[sessionID]; ok {
		return msgs
	}
	return []message.Message{}
}

// MockEmbedder is a simple embedder that returns deterministic embeddings for testing.
// It produces consistent, hash-like embeddings based on text length and content.
type MockEmbedder struct {
	dims int
}

// NewMockEmbedder creates a new mock embedder with the specified dimensions.
func NewMockEmbedder(dims int) *MockEmbedder {
	return &MockEmbedder{dims: dims}
}

// Embed converts text to a deterministic vector embedding.
// The embedding is based on text length and first character for reproducibility.
func (m *MockEmbedder) Embed(_ context.Context, text string) (embedding.Vector, error) {
	vec := make(embedding.Vector, m.dims)
	for i := range vec {
		vec[i] = float32(len(text)+i) / 100.0
		if text != "" {
			vec[i] += float32(text[0]) / 1000.0
		}
	}
	return vec, nil
}

// EmbedBatch converts multiple texts to vector embeddings.
func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([]embedding.Vector, error) {
	result := make([]embedding.Vector, len(texts))
	for i, text := range texts {
		vec, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		result[i] = vec
	}
	return result, nil
}

// Dimensions returns the dimensionality of embeddings produced by this embedder.
func (m *MockEmbedder) Dimensions() int {
	return m.dims
}
