# Type-Safe State Updates with UpdateBuilder

This example demonstrates type-safe state management using the `UpdateBuilder` API, which provides compile-time guarantees for state updates.

## Features

### Compile-Time Type Safety

```go
counterKey := state.NewKey[int]("counter", 0)
builder := state.NewUpdateBuilder()

// ✓ Type-safe: int matches Key[int]
state.SetUpdate(builder, counterKey, 42)

// ✗ Compile error: string doesn't match Key[int]
// state.SetUpdate(builder, counterKey, "wrong")
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
builder := state.NewUpdateBuilder()
state.AppendUpdate(builder, messagesKey, "value") // ✓ No typos possible
```

### Duplicate Key Detection

```go
builder := state.NewUpdateBuilder()
state.SetUpdate(builder, counterKey, 1)
state.SetUpdate(builder, counterKey, 2) // Duplicate!

updates, err := builder.Build()
// Returns error: "duplicate key \"counter\" in updates"
```

### Type-Safe Append Operations

```go
messagesKey := state.NewListKey[string]("messages", 100)

// Compile-time check ensures all values match the list type
state.AppendUpdate(builder, messagesKey, "msg1", "msg2", "msg3")

// ✗ Compile error if types don't match
// state.AppendUpdate(builder, messagesKey, 123) 
```

## Comparison

### Before: Raw Updates Map

```go
func myNode(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    return state.Updates{
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

### After: UpdateBuilder

```go
func myNode(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    builder := state.NewUpdateBuilder()
    state.SetUpdate(builder, counterKey, 42)          // ✓ Type-checked
    state.AppendUpdate(builder, messagesKey, "hello") // ✓ Type-checked
    state.AppendUpdate(builder, itemsKey, 1, 2, 3)    // ✓ Type-checked
    return builder.Build()
}
```

**Benefits:**
- ✅ Compile-time type safety
- ✅ IDE autocomplete
- ✅ Typo prevention
- ✅ Easy refactoring

## Migration Guide

### Step 1: Define Typed Keys

```go
// Instead of using raw strings:
// "counter", "status", "messages"

// Define typed keys:
counterKey := state.NewKey[int]("counter", 0)
statusKey := state.NewKey[string]("status", "")
messagesKey := state.NewListKey[string]("messages", 100)
```

### Step 2: Use UpdateBuilder

```go
// Old:
return state.Updates{
    "counter": 42,
    "status": "ok",
    "messages": []string{"hello"},
}, nil

// New:
builder := state.NewUpdateBuilder()
state.SetUpdate(builder, counterKey, 42)
state.SetUpdate(builder, statusKey, "ok")
state.AppendUpdate(builder, messagesKey, "hello")
return builder.Build()
```

### Step 3: Register Keys (if not already done)

```go
mgr := state.NewManager()
state.RegisterKey(mgr, counterKey)
state.RegisterKey(mgr, statusKey)
state.RegisterListKey(mgr, messagesKey)
```

## API Reference

### Creating Builders

```go
builder := state.NewUpdateBuilder()
```

### Setting Values

```go
state.SetUpdate[T](builder, key Key[T], value T) *UpdateBuilder
```

### Appending to Lists

```go
state.AppendUpdate[T](builder, key ListKey[T], values ...T) *UpdateBuilder
```

### Building Updates

```go
updates, err := builder.Build() // Returns error if validation fails
updates := builder.MustBuild()   // Panics on error (use in tests)
```

### Raw Updates (Escape Hatch)

```go
builder.SetRaw("dynamic_key", value) // When you don't have a typed key
```

### Deletion

```go
builder.Delete("key_to_remove") // Mark key for deletion
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
