package state

// Key represents a type-safe key for state access.
// The generic parameter T ensures compile-time type safety for Get/Set operations.
type Key[T any] struct {
	name string
	zero T // Default value returned when key doesn't exist
}

// NewKey creates a new type-safe key with a default value.
//
// Example:
//
//	var CounterKey = NewKey[int]("counter", 0)
//	var NameKey = NewKey[string]("name", "")
func NewKey[T any](name string, defaultValue T) Key[T] {
	return Key[T]{name: name, zero: defaultValue}
}

// Name returns the key's name.
func (k Key[T]) Name() string {
	return k.name
}

// Zero returns the key's default value.
func (k Key[T]) Zero() T {
	return k.zero
}

// ListKey represents a type-safe key for list values with optional max size.
type ListKey[T any] struct {
	Key[[]T]
	maxSize int
}

// NewListKey creates a new type-safe list key with a maximum size.
// If maxSize is 0, the list can grow unbounded.
// If maxSize > 0, the list will be truncated to keep only the last maxSize elements.
//
// Example:
//
//	var MessagesKey = NewListKey[message.Message]("messages", 100)
func NewListKey[T any](name string, maxSize int) ListKey[T] {
	return ListKey[T]{
		Key:     NewKey[[]T](name, nil),
		maxSize: maxSize,
	}
}

// MaxSize returns the maximum number of elements in the list.
func (k ListKey[T]) MaxSize() int {
	return k.maxSize
}
