package state

// Reader provides read-only access to graph state for deterministic node execution.
// Nodes receive this interface to prevent direct state mutations, ensuring all updates
// go through the NodeResult return value for atomic application between supersteps.
//
// Use Case: Pass to node RunFunc for read-only state access.
type Reader interface {
	// Get retrieves the current value from a named channel.
	Get(key string) any

	// GetAll returns a snapshot of all channel values.
	GetAll() map[string]any

	// MessagesSnapshot returns the execution history from the "messages" channel.
	// Each ExecutionResult contains a message with metadata (node, timestamp, updates).
	MessagesSnapshot() []ExecutionResult

	// AggregatesSnapshot returns a copy of global aggregates from the previous superstep.
	AggregatesSnapshot() map[string]any

	// Type-safe accessor methods to prevent runtime panics

	// GetString retrieves a string value from state.
	// Returns error if key doesn't exist or value is not a string.
	GetString(key string) (string, error)

	// GetInt retrieves an integer value from state.
	// Returns error if key doesn't exist or value is not an int.
	GetInt(key string) (int, error)

	// GetInt64 retrieves an int64 value from state.
	// Returns error if key doesn't exist or value is not an int64.
	GetInt64(key string) (int64, error)

	// GetFloat64 retrieves a float64 value from state.
	// Returns error if key doesn't exist or value is not a float64.
	GetFloat64(key string) (float64, error)

	// GetBool retrieves a boolean value from state.
	// Returns error if key doesn't exist or value is not a bool.
	GetBool(key string) (bool, error)

	// GetSlice retrieves a slice value from state.
	// Returns error if key doesn't exist or value is not a slice.
	GetSlice(key string) ([]any, error)

	// GetMap retrieves a map value from state.
	// Returns error if key doesn't exist or value is not a map.
	GetMap(key string) (map[string]any, error)
}

// Writer extends Reader with write capabilities for state mutations.
// This allows nodes to update state values and contribute to aggregators.
//
// Use Case: Pass to node RunFunc when write access is needed.
type Writer interface {
	Reader

	// Set updates or creates a channel value.
	Set(key string, value any) error

	// Aggregate contributes a value to a named aggregator for the current superstep.
	Aggregate(name string, value any) error
}
