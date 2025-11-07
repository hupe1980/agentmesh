package graph

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// =============================================================================
// State Interfaces
// =============================================================================

// StateReader provides read-only access to graph state for deterministic node execution.
// Nodes receive this interface to prevent direct state mutations, ensuring all updates
// go through the NodeResult return value for atomic application between supersteps.
type StateReader interface {
	// Get retrieves the current value from a named channel.
	Get(key string) any

	// GetAll returns a snapshot of all channel values.
	GetAll() map[string]any

	// MessagesSnapshot returns messages from the "messages" channel.
	MessagesSnapshot() []message.Message

	// AggregatesSnapshot returns a copy of global aggregates from the previous superstep.
	AggregatesSnapshot() map[string]any
}

// StateWriter extends StateReader with mutation capabilities for aggregators.
type StateWriter interface {
	StateReader

	// Aggregate contributes a value to a named aggregator for the current superstep.
	Aggregate(name string, value any) error
}

// =============================================================================
// StateManager Interface
// =============================================================================

// StateManager owns all state concerns: channels, checkpoints, and aggregates.
// This provides a clean separation of state management from execution logic.
//
// Responsibilities:
//   - Channel-based state management (TopicChannel, LastValueChannel, BinaryOpChannel)
//   - Checkpoint persistence and restoration
//   - Aggregate value management (cross-node reductions)
//   - Thread-safe state access and mutations
//
// Design Goals:
//   - Single source of truth for all graph state
//   - Clean interface for state reads and writes
//   - Pluggable checkpoint backends
//   - No execution logic (pure state management)
type StateManager interface {
	// State access
	Get(key string) any
	GetAll() map[string]any
	MessagesSnapshot() []message.Message
	AggregatesSnapshot() map[string]any

	// Channel management
	AddChannel(ch channel.Channel)
	GetChannel(name string) (channel.Channel, bool)
	UpdateChannel(ctx context.Context, name string, value any) error
	UpdateChannels(ctx context.Context, updates map[string]any) error

	// Aggregate management
	GetAggregate(name string) any
	GetAggregatesSnapshot() map[string]any
	SetAggregates(aggregates map[string]any)
	SetAggregateFn(fn func(string, any) error)
	RecordAggregation(name string, value any) error

	// Checkpoint management
	SaveCheckpoint(ctx context.Context, runID string, superstep int64, metadata map[string]any) error
	LoadCheckpoint(ctx context.Context, runID string) (*checkpoint.Checkpoint, error)
	SetCheckpointer(checkpointer checkpoint.Checkpointer)

	// Message management
	AddMessages(messages []message.Message)
	ApplyUpdates(values map[string]any, messages []message.Message)
	Set(key string, value any) error

	// State snapshots
	Snapshot() map[string]any
	Clone() StateManager
}

// =============================================================================
// GraphState - Primary StateManager Implementation
// =============================================================================

// GraphState is the primary implementation of the StateManager interface.
// It manages data flow through typed channels with thread-safe access.
//
// GraphState serves as both:
// - The user-facing API for building graphs (via NewGraphState, Graph.State)
// - The runtime state during execution (no conversion needed)
//
// This is the ONLY StateManager implementation in v2.0+ (Option A architecture).
type GraphState struct {
	channels     *channel.ChannelSet
	aggregates   map[string]any
	aggregateFn  func(string, any) error
	aggregatesMu sync.RWMutex
	checkpointer checkpoint.Checkpointer
	mu           sync.RWMutex // Protects channels and overall state
}

// NewGraphState creates a new channel-based graph state.
// It automatically creates a standard "messages" channel (Topic with maxMessages limit).
func NewGraphState(maxMessages int) *GraphState {
	channels := channel.NewChannelSet()
	channels.Add(channel.NewTopicChannel("messages", maxMessages))
	return &GraphState{
		channels:   channels,
		aggregates: make(map[string]any),
	}
}

// =============================================================================
// GraphState - State Read Methods
// =============================================================================

func (s *GraphState) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ch, ok := s.channels.Get(key)
	if !ok {
		return nil
	}
	val, err := ch.Read(context.Background())
	if err != nil {
		return nil
	}
	return val
}

func (s *GraphState) GetAll() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	values, err := s.channels.ReadAll(context.Background())
	if err != nil {
		return nil
	}
	return values
}

func (s *GraphState) MessagesSnapshot() []message.Message {
	ch, ok := s.GetChannel("messages")
	if !ok {
		return nil
	}

	val, err := ch.Read(context.Background())
	if err != nil || val == nil {
		return nil
	}

	// Convert []any to []message.Message
	values, ok := val.([]any)
	if !ok || len(values) == 0 {
		return nil
	}

	messages := make([]message.Message, 0, len(values))
	for _, v := range values {
		if msg, ok := v.(message.Message); ok {
			messages = append(messages, msg)
		}
	}
	return messages
}

// =============================================================================
// GraphState - Channel Management
// =============================================================================

func (s *GraphState) AddChannel(ch channel.Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels.Add(ch)
}

func (s *GraphState) GetChannel(name string) (channel.Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.channels.Get(name)
}

func (s *GraphState) UpdateChannel(ctx context.Context, name string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch, ok := s.channels.Get(name)
	if !ok {
		return nil // Silently ignore unknown channels
	}
	return ch.Write(ctx, value)
}

func (s *GraphState) UpdateChannels(ctx context.Context, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for name, value := range updates {
		ch, ok := s.channels.Get(name)
		if !ok {
			continue // Skip unknown channels
		}
		if err := ch.Write(ctx, value); err != nil {
			return fmt.Errorf("failed to write to channel %q: %w", name, err)
		}
	}
	return nil
}

// =============================================================================
// GraphState - Aggregate Management
// =============================================================================

func (s *GraphState) GetAggregate(name string) any {
	s.aggregatesMu.RLock()
	defer s.aggregatesMu.RUnlock()
	return s.aggregates[name]
}

func (s *GraphState) GetAggregatesSnapshot() map[string]any {
	s.aggregatesMu.RLock()
	defer s.aggregatesMu.RUnlock()

	if len(s.aggregates) == 0 {
		return nil
	}

	snapshot := make(map[string]any, len(s.aggregates))
	for k, v := range s.aggregates {
		snapshot[k] = v
	}
	return snapshot
}

func (s *GraphState) AggregatesSnapshot() map[string]any {
	return s.GetAggregatesSnapshot()
}

func (s *GraphState) SetAggregates(aggregates map[string]any) {
	s.aggregatesMu.Lock()
	defer s.aggregatesMu.Unlock()

	if len(aggregates) == 0 {
		s.aggregates = nil
		return
	}

	if s.aggregates == nil {
		s.aggregates = make(map[string]any, len(aggregates))
	} else {
		for k := range s.aggregates {
			delete(s.aggregates, k)
		}
	}

	for k, v := range aggregates {
		s.aggregates[k] = v
	}
}

func (s *GraphState) SetAggregateFn(fn func(string, any) error) {
	s.aggregatesMu.Lock()
	defer s.aggregatesMu.Unlock()
	s.aggregateFn = fn
}

func (s *GraphState) RecordAggregation(name string, value any) error {
	s.aggregatesMu.RLock()
	fn := s.aggregateFn
	s.aggregatesMu.RUnlock()

	if fn == nil {
		return ErrAggregatorsNotConfigured
	}
	return fn(name, value)
}

func (s *GraphState) Aggregate(name string, value any) error {
	return s.RecordAggregation(name, value)
}

// =============================================================================
// GraphState - Checkpoint Management
// =============================================================================

func (s *GraphState) SaveCheckpoint(ctx context.Context, runID string, superstep int64, metadata map[string]any) error {
	s.mu.RLock()
	checkpointer := s.checkpointer
	s.mu.RUnlock()

	if checkpointer == nil {
		return nil // No checkpointer configured
	}

	snapshot := s.Snapshot()
	return checkpointer.Save(ctx, &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: superstep,
		State:     snapshot,
		Metadata:  metadata,
	})
}

func (s *GraphState) LoadCheckpoint(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	s.mu.RLock()
	checkpointer := s.checkpointer
	s.mu.RUnlock()

	if checkpointer == nil {
		return nil, fmt.Errorf("no checkpointer configured")
	}

	return checkpointer.Load(ctx, runID)
}

func (s *GraphState) SetCheckpointer(checkpointer checkpoint.Checkpointer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkpointer = checkpointer
}

// =============================================================================
// GraphState - Message Management
// =============================================================================

func (s *GraphState) AddMessages(messages []message.Message) {
	if len(messages) == 0 {
		return
	}

	ch, ok := s.GetChannel("messages")
	if !ok {
		return
	}

	values := make([]any, len(messages))
	for i, msg := range messages {
		values[i] = msg
	}

	_ = ch.Write(context.Background(), values) // Ignore error - internal initialization
}

// SetMaxMessages updates the retention limit of the "messages" channel without
// discarding existing channel configuration.
func (s *GraphState) SetMaxMessages(maxMessages int) {
	if maxMessages < 0 {
		maxMessages = 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ch, ok := s.channels.Get("messages")
	if !ok {
		s.channels.Add(channel.NewTopicChannel("messages", maxMessages))
		return
	}

	if topic, ok := ch.(*channel.TopicChannel); ok {
		topic.SetMaxValues(maxMessages)
		return
	}

	// Fallback: replace with a new topic channel while attempting to preserve messages.
	snapshot, err := ch.Read(context.Background())
	if err != nil {
		snapshot = nil
	}

	newTopic := channel.NewTopicChannel("messages", maxMessages)
	if values, ok := snapshot.([]any); ok && len(values) > 0 {
		_ = newTopic.Write(context.Background(), values)
	}

	s.channels.Add(newTopic)
}

func (s *GraphState) ApplyUpdates(values map[string]any, messages []message.Message) {
	ctx := context.Background()

	s.mu.Lock()
	defer s.mu.Unlock()

	for key, value := range values {
		ch, exists := s.channels.Get(key)
		if !exists {
			ch = channel.NewLastValueChannel(key)
			s.channels.Add(ch)
		}
		_ = ch.Write(ctx, value) // Ignore error - updates are best-effort
	}

	if len(messages) > 0 {
		if ch, ok := s.channels.Get("messages"); ok {
			vals := make([]any, len(messages))
			for i, msg := range messages {
				vals[i] = msg
			}
			_ = ch.Write(ctx, vals) // Ignore error - updates are best-effort
		}
	}
}

func (s *GraphState) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch, exists := s.channels.Get(key)
	if !exists {
		ch = channel.NewLastValueChannel(key)
		s.channels.Add(ch)
	}
	return ch.Write(context.Background(), value)
}

// =============================================================================
// GraphState - State Snapshots
// =============================================================================

func (s *GraphState) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	values, err := s.channels.SnapshotAll(context.Background())
	if err != nil {
		return nil
	}
	return values
}

func (s *GraphState) Clone() StateManager {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create new state with same channel configuration
	cloned := &GraphState{
		channels:     channel.NewChannelSet(),
		aggregates:   make(map[string]any),
		checkpointer: s.checkpointer,
	}

	// Clone all channels
	for _, name := range s.channels.List() {
		if ch, ok := s.channels.Get(name); ok {
			cloned.channels.Add(ch.Clone())
		}
	}

	// Copy aggregates
	s.aggregatesMu.RLock()
	maps.Copy(cloned.aggregates, s.aggregates)
	cloned.aggregateFn = s.aggregateFn
	s.aggregatesMu.RUnlock()

	return cloned
}

// =============================================================================
// GraphState - Legacy Methods (for v1.x compatibility)
// =============================================================================

func (s *GraphState) SnapshotAll() map[string]any {
	return s.Snapshot() // Alias
}

func (s *GraphState) ListChannels() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.channels.List()
}

// Ensure GraphState implements StateManager interface
var _ StateManager = (*GraphState)(nil)

// =============================================================================
// State Adapters
// =============================================================================

// StateReaderAdapter adapts StateManager to StateReader interface.
// This provides a read-only view of the state, preventing nodes from
// directly mutating state outside of the BSP model (updates must go
// through NodeResult).
type StateReaderAdapter struct {
	manager StateManager
}

// NewStateReaderAdapter creates a read-only view of a StateManager.
// Nodes receive this interface to prevent direct state mutations.
func NewStateReaderAdapter(manager StateManager) *StateReaderAdapter {
	return &StateReaderAdapter{manager: manager}
}

func (sr *StateReaderAdapter) Get(key string) any {
	return sr.manager.Get(key)
}

func (sr *StateReaderAdapter) GetAll() map[string]any {
	return sr.manager.GetAll()
}

func (sr *StateReaderAdapter) MessagesSnapshot() []message.Message {
	return sr.manager.MessagesSnapshot()
}

func (sr *StateReaderAdapter) AggregatesSnapshot() map[string]any {
	return sr.manager.GetAggregatesSnapshot()
}

// StateWriterAdapter adapts StateManager to StateWriter interface.
// This extends StateReader with aggregation capabilities, allowing
// nodes to contribute to global aggregators during execution.
type StateWriterAdapter struct {
	*StateReaderAdapter
	manager StateManager
}

// NewStateWriterAdapter creates a read-write view of a StateManager.
// This is the interface that nodes receive during execution, providing
// read access to state and write access to aggregators.
func NewStateWriterAdapter(manager StateManager) *StateWriterAdapter {
	return &StateWriterAdapter{
		StateReaderAdapter: NewStateReaderAdapter(manager),
		manager:            manager,
	}
}

func (sw *StateWriterAdapter) Aggregate(name string, value any) error {
	return sw.manager.RecordAggregation(name, value)
}

// =============================================================================
// bufferedStateWriter - Internal BSP helper
// =============================================================================

// bufferedStateWriter wraps a StateReader and buffers all Aggregate() calls.
// This ensures mutations are not visible within the same superstep, maintaining
// Pregel's BSP (Bulk Synchronous Parallel) semantics where all updates become
// visible only after the superstep barrier.
//
// Without buffering, aggregates would be immediately visible to other vertices
// in the same superstep, breaking the BSP model's guarantee of deterministic
// execution order independence.
//
// The buffered aggregates are flushed at the end of node execution and applied
// to the runtime's aggregator state for the next superstep.
type bufferedStateWriter struct {
	reader            StateReader
	pendingAggregates map[string][]any
	mu                sync.Mutex
}

func newBufferedStateWriter(reader StateReader) *bufferedStateWriter {
	return &bufferedStateWriter{
		reader:            reader,
		pendingAggregates: make(map[string][]any),
	}
}

func (bsw *bufferedStateWriter) Get(key string) any {
	return bsw.reader.Get(key)
}

func (bsw *bufferedStateWriter) GetAll() map[string]any {
	return bsw.reader.GetAll()
}

func (bsw *bufferedStateWriter) MessagesSnapshot() []message.Message {
	return bsw.reader.MessagesSnapshot()
}

func (bsw *bufferedStateWriter) AggregatesSnapshot() map[string]any {
	return bsw.reader.AggregatesSnapshot()
}

func (bsw *bufferedStateWriter) Aggregate(name string, value any) error {
	bsw.mu.Lock()
	defer bsw.mu.Unlock()

	if name == "" {
		return fmt.Errorf("aggregate name cannot be empty")
	}

	bsw.pendingAggregates[name] = append(bsw.pendingAggregates[name], value)
	return nil
}

func (bsw *bufferedStateWriter) flushAggregates() map[string][]any {
	bsw.mu.Lock()
	defer bsw.mu.Unlock()

	if len(bsw.pendingAggregates) == 0 {
		return nil
	}

	flushed := make(map[string][]any, len(bsw.pendingAggregates))
	for k, v := range bsw.pendingAggregates {
		copied := make([]any, len(v))
		copy(copied, v)
		flushed[k] = copied
	}
	bsw.pendingAggregates = make(map[string][]any)

	return flushed
}

func (bsw *bufferedStateWriter) resetAggregates() {
	bsw.mu.Lock()
	defer bsw.mu.Unlock()
	if len(bsw.pendingAggregates) == 0 {
		return
	}
	bsw.pendingAggregates = make(map[string][]any)
}

// =============================================================================
// Helper functions
// =============================================================================

// cloneMessages creates a deep copy of a message slice for immutability.
func cloneMessages(msgs []message.Message) []message.Message {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]message.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, msg.Clone())
	}
	return out
}

// ensureGraphState creates a new GraphState if the provided state is nil.
func ensureGraphState(state *GraphState) *GraphState {
	if state == nil {
		return NewGraphState(0) // Unlimited messages by default
	}
	return state
}
