package state

import (
	"fmt"
	"reflect"
	"sync"
)

// TypeInfo holds metadata about a registered key type.
type TypeInfo struct {
	// KeyName is the string identifier for the key (e.g., "messages", "user_id")
	KeyName string
	// ValueType is the reflect.Type of the value stored at this key
	ValueType reflect.Type
	// IsList indicates if this is a ListKey (topic/append semantics) vs Key (last-value semantics)
	IsList bool
}

// TypeRegistry provides runtime type validation for state keys.
// When keys are registered, their types are recorded. On subsequent access,
// the registry validates that the requested type matches the registered type.
//
// This provides type safety at the boundary between compile-time (generic Key[T])
// and runtime (untyped channel storage), catching type mismatches early.
type TypeRegistry struct {
	types map[string]*TypeInfo
	mu    sync.RWMutex
}

// NewTypeRegistry creates a new type registry.
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		types: make(map[string]*TypeInfo),
	}
}

// RegisterKey records the type information for a key.
// If the key is already registered with a different type, returns an error.
func (tr *TypeRegistry) RegisterKey(keyName string, valueType reflect.Type, isList bool) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if existing, exists := tr.types[keyName]; exists {
		// Check if registration matches existing type
		if existing.ValueType != valueType {
			return fmt.Errorf(
				"type mismatch for key %q: registered as %s, attempted to register as %s",
				keyName, existing.ValueType, valueType,
			)
		}
		if existing.IsList != isList {
			listStr := map[bool]string{true: "ListKey", false: "Key"}
			return fmt.Errorf(
				"semantics mismatch for key %q: registered as %s, attempted to register as %s",
				keyName, listStr[existing.IsList], listStr[isList],
			)
		}
		// Already registered with same type, no-op
		return nil
	}

	tr.types[keyName] = &TypeInfo{
		KeyName:   keyName,
		ValueType: valueType,
		IsList:    isList,
	}
	return nil
}

// ValidateType checks if a value matches the registered type for a key.
// Returns an error if:
//   - The key is not registered
//   - The value type doesn't match the registered type
//   - For list keys, the value is not a slice of the registered element type
func (tr *TypeRegistry) ValidateType(keyName string, value any) error {
	tr.mu.RLock()
	typeInfo, exists := tr.types[keyName]
	tr.mu.RUnlock()

	if !exists {
		return fmt.Errorf("key %q not registered in type registry", keyName)
	}

	valueType := reflect.TypeOf(value)
	if valueType == nil {
		// nil value - check if registered type is a pointer or interface
		if typeInfo.ValueType.Kind() == reflect.Ptr || typeInfo.ValueType.Kind() == reflect.Interface {
			return nil
		}
		return fmt.Errorf("nil value for non-pointer type %s", typeInfo.ValueType)
	}

	// For list keys, value should be a slice of the element type
	if typeInfo.IsList {
		if valueType.Kind() != reflect.Slice {
			return fmt.Errorf(
				"type mismatch for list key %q: expected slice, got %s",
				keyName, valueType,
			)
		}
		elemType := valueType.Elem()
		if elemType != typeInfo.ValueType {
			return fmt.Errorf(
				"type mismatch for list key %q: expected []%s, got []%s",
				keyName, typeInfo.ValueType, elemType,
			)
		}
		return nil
	}

	// For regular keys, check direct type match
	if !valueType.AssignableTo(typeInfo.ValueType) {
		return fmt.Errorf(
			"type mismatch for key %q: expected %s, got %s",
			keyName, typeInfo.ValueType, valueType,
		)
	}

	return nil
}

// GetRegisteredType retrieves type information for a key.
// Returns nil if the key is not registered.
func (tr *TypeRegistry) GetRegisteredType(keyName string) *TypeInfo {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return tr.types[keyName]
}

// IsRegistered checks if a key has been registered.
func (tr *TypeRegistry) IsRegistered(keyName string) bool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	_, exists := tr.types[keyName]
	return exists
}

// RegisteredKeys returns a list of all registered key names.
func (tr *TypeRegistry) RegisteredKeys() []string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	keys := make([]string, 0, len(tr.types))
	for key := range tr.types {
		keys = append(keys, key)
	}
	return keys
}

// Clear removes all registered types (primarily for testing).
func (tr *TypeRegistry) Clear() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.types = make(map[string]*TypeInfo)
}

// Len returns the number of registered types.
func (tr *TypeRegistry) Len() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return len(tr.types)
}

// MustRegisterKey is like RegisterKey but panics on error.
// Useful for initialization code where type mismatches are programming errors.
func (tr *TypeRegistry) MustRegisterKey(keyName string, valueType reflect.Type, isList bool) {
	if err := tr.RegisterKey(keyName, valueType, isList); err != nil {
		panic(fmt.Sprintf("failed to register key: %v", err))
	}
}

// MustValidateType is like ValidateType but panics on error.
// Useful for scenarios where type mismatches indicate serious bugs.
func (tr *TypeRegistry) MustValidateType(keyName string, value any) {
	if err := tr.ValidateType(keyName, value); err != nil {
		panic(fmt.Sprintf("type validation failed: %v", err))
	}
}

// IsListKey checks if a key is registered as a ListKey (append semantics).
// Returns false if the key is not registered or is a regular Key.
func (tr *TypeRegistry) IsListKey(keyName string) bool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if info, exists := tr.types[keyName]; exists {
		return info.IsList
	}
	return false
}
