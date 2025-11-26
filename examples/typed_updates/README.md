# Type-Safe State Updates

This example demonstrates type-safe state management using typed keys with Go generics, which provides compile-time guarantees for state updates.

## Features

### Compile-Time Type Safety

```go
counterKey := state.NewKey[int]("counter", 0)

// ✓ Type-safe: int matches Key[int]
updates := state.Updates{}
updates[counterKey.Name()] = 42

// ✗ Compile error: string doesn't match Key[int]
// updates[counterKey.Name()] = "wrong"  // IDE will show type error
```

### Typo Prevention

Instead of string keys that can be mistyped:

```go
// Old way - runtime errors from typos
return state.Updates{
    "mesages": value, // Typo! Won't be caught until runtime
}, nil
```

Use typed keys:

```go
// New way - compile-time checking
messagesKey := state.NewListKey[string]("messages", 100)
updates := state.Updates{}
updates[messagesKey.Name()] = []string{"value"} // ✓ No typos possible
```

### Type-Safe Operations

```go
counterKey := state.NewKey[int]("counter", 0)
messagesKey := state.NewListKey[string]("messages", 100)

func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Read with type safety
    counter := state.GetFromView(view, counterKey)  // Returns int
    messages := state.GetFromView(view, messagesKey) // Returns []string
    
    // Update with type safety
    updates := state.Updates{}
    updates[counterKey.Name()] = counter + 1      // Must be int
    updates[messagesKey.Name()] = []string{"new"} // Must be []string
    
    return []string{"next"}, updates, nil
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

### With Typed Keys (Current API)

```go
var (
    counterKey  = state.NewKey[int]("counter", 0)
    messagesKey = state.NewListKey[string]("messages", 100)
    itemsKey    = state.NewListKey[int]("items", 100)
)

func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Type-safe reads - no casts needed
    counter := state.GetFromView(view, counterKey) // Returns int
    
    // Type-safe updates
    updates := state.Updates{}
    updates[counterKey.Name()] = 42                  // ✓ Type-checked
    updates[messagesKey.Name()] = []string{"hello"}  // ✓ Type-checked
    updates[itemsKey.Name()] = []int{1, 2, 3}        // ✓ Type-checked
    
    return []string{"next"}, updates, nil
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
