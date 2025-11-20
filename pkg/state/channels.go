package state

import (
	"github.com/hupe1980/agentmesh/pkg/state/internal/channel"
)

// Channel interfaces and types for state management.
// Implementation is in internal/channel to prevent direct usage outside the state package.

// Channel is the user-facing abstraction for data flow between nodes.
// Provides simple read/write operations with channel-specific update semantics.
type Channel = channel.Channel

// ResettableChannel extends Channel with administrative operations.
// Provides dangerous state-clearing operations that should only be used
// with explicit understanding of the consequences.
type ResettableChannel = channel.ResettableChannel

// SliceValue is an interface for values that can be treated as slices.
type SliceValue = channel.SliceValue

// SliceOf is a generic helper that wraps any slice type to implement SliceValue.
type SliceOf[T any] = channel.SliceOf[T]

// TopicChannel accumulates values in a list with append-only semantics.
// Each write operation adds to the channel without replacing previous values.
type TopicChannel = channel.TopicChannel

// LastValueChannel stores only the most recent value with overwrite semantics.
// Each write operation replaces the previous value completely.
type LastValueChannel = channel.LastValueChannel

// BinaryOpChannel merges values using a custom binary operator function.
// Useful for implementing reducers, aggregations, and custom merge logic.
type BinaryOpChannel = channel.BinaryOpChannel

// AggregateChannel implements cross-node aggregation with zero values and merge operations.
// Designed for distributed coordination and global state accumulation.
type AggregateChannel = channel.AggregateChannel

// Aggregator combines per-vertex contributions into a single value.
type Aggregator = channel.Aggregator

// StringSet is a thread-safe set of strings.
type StringSet = channel.Set

// Errors
var (
	ErrNilValue = channel.ErrNilValue
)

// Constructor functions
var (
	NewTopicChannel     = channel.NewTopicChannel
	NewLastValueChannel = channel.NewLastValueChannel
	NewBinaryOpChannel  = channel.NewBinaryOpChannel
	NewAggregateChannel = channel.NewAggregateChannel
	NewStringSet        = channel.NewSet
)
