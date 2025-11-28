# Type-Safe State Updates

This example demonstrates **type-safe state builders** using `UpdateBuilder` and generic helper functions with Go generics for compile-time guarantees.

## Features

### Two Builder APIs

AgentMesh provides **two complementary builder APIs**:

1. **`state.UpdateBuilder`** - For standalone state construction
2. **`command.Command`** - For node functions with routing

Both eliminate manual map construction and provide compile-time type safety.

### 1. state.UpdateBuilder - Basic Updates

```go
counterKey := state.NewKey[int]("counter", 0)

// Build updates with fluent API using With() and SetValue()
updates := state.NewUpdateBuilder().
    With(state.SetValue(counterKey, 42)).
    Build()  // Returns state.Updates

// ✗ Compile error: string doesn't match Key[int]
// .With(state.SetValue(counterKey, "wrong"))  // IDE shows type error immediately
```

### 2. Type-Safe List Operations

For `ListKey[T]`, use `AppendValue` helper that automatically wraps values in `SliceOf[T]`:

```go
messagesKey := state.NewListKey[string]("messages", 100)

// Append single value
updates := state.NewUpdateBuilder().
    With(state.AppendValue(messagesKey, "New message")).
    Build()

// Append multiple values (variadic)
updates := state.NewUpdateBuilder().
    With(state.AppendValue(messagesKey, "msg1", "msg2")).
    Build()

// ✗ Compile error: int doesn't match ListKey[string]
// state.AppendValue(messagesKey, 123)  // Type mismatch caught at compile time
```

### 3. command.Command - Node Functions with Routing

Use `command.Command` for returning `([]string, state.Updates, error)` from node functions:

```go
import "github.com/hupe1980/agentmesh/pkg/command"

func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Read with type safety
    counter := state.GetFromView(view, counterKey)  // Returns int
    
    // Update with Command builder (fully type-safe)
    return command.New().
        With(command.SetValue(counterKey, counter + 1)).
        To("next_node")  // Returns full tuple
}

// For list operations in nodes
func appendNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    return command.New().With(command.Append(messagesKey, "New message")).To("next")
}
```

## Comparison

### Without Type Safety (Raw Strings)

```go
func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // No IDE autocomplete, no type checking
    counter := view.Get("counter").(int)  // Runtime cast - risky!
    
    return []string{"next"}, state.Updates{
        "counter": 42,              // No type checking
        "mesages": "typo",          // Typo not caught!
        "items": []int{1, 2, 3},    // No compile-time verification
    }, nil
}
```

**Problems:**
- ❌ No type safety - any value can go anywhere
- ❌ Typos cause runtime errors
- ❌ No IDE autocomplete
- ❌ Hard to refactor (find all usages)
- ❌ Runtime casts required

### With Type-Safe Builders (New API)

```go
var (
    counterKey  = state.NewKey[int]("counter", 0)
    messagesKey = state.NewListKey[string]("messages", 100)
    itemsKey    = state.NewListKey[int]("items", 100)
)

func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Type-safe reads - no casts needed
    counter := state.GetFromView(view, counterKey) // Returns int
    
    // Type-safe updates with UpdateBuilder
    return []string{"next"}, state.NewUpdateBuilder().
        With(state.SetValue(counterKey, 42)).  // ✓ Type-checked at compile time
        Build(), nil
    
    // Or use Command for cleaner syntax with compile-time type checking
    return command.New().
        With(command.SetValue(counterKey, 42)).
        To("next")  // Returns ([]string, state.Updates, error)
}

func appendNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Type-safe list operations with helpers
    return command.New().With(command.Append(messagesKey, "hello")).To("next")  // ✓ Type-checked
}
```

**Benefits:**
- ✅ Compile-time type safety
- ✅ IDE autocomplete
- ✅ Typo prevention
- ✅ Easy refactoring
- ✅ No runtime casts needed

## Usage Guide

### Step 1: Define Typed Keys

```go
// Define typed keys with default values
var (
    counterKey  = state.NewKey[int]("counter", 0)
    statusKey   = state.NewKey[string]("status", "")
    messagesKey = state.NewListKey[string]("messages", 100)
)
```

### Step 2: Register Keys with State Manager

```go
mgr := state.NewManager()
state.RegisterKey(mgr, counterKey)
state.RegisterKey(mgr, statusKey)
state.RegisterListKey(mgr, messagesKey)
```

### Step 3: Use in Node Functions

```go
func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Read values with type safety
    counter := state.GetFromView(view, counterKey)   // int
    status := state.GetFromView(view, statusKey)     // string
    messages := state.GetFromView(view, messagesKey) // []string
    
    // Create updates with type safety
    updates := state.Updates{}
    updates[counterKey.Name()] = counter + 1
    updates[statusKey.Name()] = "processing"
    updates[messagesKey.Name()] = append(messages, "new message")
    
    return []string{"next"}, updates, nil
}
```

## API Reference

### Creating Typed Keys

```go
// Simple keys
key := state.NewKey[T](name string, defaultValue T) Key[T]

// List keys
listKey := state.NewListKey[T](name string, maxSize int) ListKey[T]
```

### Reading State

```go
value := state.GetFromView(view, key)  // Returns T
```

### Updating State

```go
updates := state.Updates{}
updates[key.Name()] = newValue        // Set value
updates[listKey.Name()] = []T{items}  // Set list
```

### Special Operations

```go
// Append to existing list
messages := state.GetFromView(view, messagesKey)
updates[messagesKey.Name()] = append(messages, "new")

// Delete a key
updates[key.Name()] = nil  // or omit the key
```

## Running the Example

```bash
go run main.go
```

## Key Takeaways

1. **Type Safety**: Catch errors at compile-time, not runtime
2. **Better IDE Support**: Autocomplete for keys and values
3. **Refactoring**: Easier to find and update all usages
4. **Documentation**: Types serve as inline documentation
5. **Validation**: Duplicate key detection at build time

## See Also

- [State Management Documentation](../../docs/state-management.md)
- [Core Concepts - State](../../docs/core-concepts.md#state)
- [Examples - State Builder](../state_builder)
