package state

import "errors"

// ErrKeyNotRegistered is returned when attempting to access an unregistered key.
var ErrKeyNotRegistered = errors.New("key not registered")

// ErrKeyNotList is returned when attempting list operations on non-list keys.
var ErrKeyNotList = errors.New("key is not a list")

// ErrTypeMismatch is returned when a value's type doesn't match the registered key type.
var ErrTypeMismatch = errors.New("type mismatch")

// ErrNodeExecution is a sentinel error that wraps node execution failures.
// Used to distinguish node-level errors from system-level errors in execution.
var ErrNodeExecution = errors.New("node execution failed")
