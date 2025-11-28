package command

import (
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestSetValue_TypeSafety(t *testing.T) {
	statusKey := state.NewKey[string]("status", "")
	countKey := state.NewKey[int]("count", 0)

	tests := []struct {
		name     string
		setup    func() *Command
		validate func(*testing.T, *Command)
	}{
		{
			name: "single value",
			setup: func() *Command {
				return New().With(SetValue(statusKey, "completed"))
			},
			validate: func(t *testing.T, cmd *Command) {
				if len(cmd.m) != 1 {
					t.Errorf("expected 1 key, got %d", len(cmd.m))
				}
				if cmd.m[statusKey.Name()] != "completed" {
					t.Errorf("expected 'completed', got %v", cmd.m[statusKey.Name()])
				}
			},
		},
		{
			name: "multiple values",
			setup: func() *Command {
				return New().
					With(SetValue(statusKey, "processing")).
					With(SetValue(countKey, 42))
			},
			validate: func(t *testing.T, cmd *Command) {
				if len(cmd.m) != 2 {
					t.Errorf("expected 2 keys, got %d", len(cmd.m))
				}
				if cmd.m[statusKey.Name()] != "processing" {
					t.Errorf("expected 'processing', got %v", cmd.m[statusKey.Name()])
				}
				if cmd.m[countKey.Name()] != 42 {
					t.Errorf("expected 42, got %v", cmd.m[countKey.Name()])
				}
			},
		},
		{
			name: "overwrite value",
			setup: func() *Command {
				return New().
					With(SetValue(statusKey, "first")).
					With(SetValue(statusKey, "second"))
			},
			validate: func(t *testing.T, cmd *Command) {
				if cmd.m[statusKey.Name()] != "second" {
					t.Errorf("expected 'second', got %v", cmd.m[statusKey.Name()])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setup()
			tt.validate(t, cmd)
		})
	}
}

func TestAppend_Variadic(t *testing.T) {
	msgKey := state.NewListKey[string]("messages", 0)

	t.Run("single value", func(t *testing.T) {
		cmd := New().With(Append(msgKey, "hello"))

		updates, err := cmd.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sv, ok := updates[msgKey.Name()].(state.SliceValue)
		if !ok {
			t.Fatalf("expected SliceValue, got %T", updates[msgKey.Name()])
		}

		slice := sv.ToSlice()
		if len(slice) != 1 {
			t.Errorf("expected 1 value, got %d", len(slice))
		}
		if slice[0] != "hello" {
			t.Errorf("expected 'hello', got %v", slice[0])
		}
	})

	t.Run("multiple values", func(t *testing.T) {
		cmd := New().With(Append(msgKey, "a", "b", "c"))

		updates, err := cmd.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sv := updates[msgKey.Name()].(state.SliceValue)
		slice := sv.ToSlice()
		if len(slice) != 3 {
			t.Errorf("expected 3 values, got %d", len(slice))
		}
		if slice[0] != "a" || slice[1] != "b" || slice[2] != "c" {
			t.Errorf("unexpected values: %v", slice)
		}
	})

	t.Run("spread slice", func(t *testing.T) {
		values := []string{"x", "y", "z"}
		cmd := New().With(Append(msgKey, values...))

		updates, err := cmd.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sv := updates[msgKey.Name()].(state.SliceValue)
		slice := sv.ToSlice()
		if len(slice) != 3 {
			t.Errorf("expected 3 values, got %d", len(slice))
		}
	})

	t.Run("multiple appends", func(t *testing.T) {
		cmd := New().
			With(Append(msgKey, "msg1")).
			With(Append(msgKey, "msg2")).
			With(Append(msgKey, "msg3"))

		updates, err := cmd.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sv := updates[msgKey.Name()].(state.SliceValue)
		slice := sv.ToSlice()
		if len(slice) != 3 {
			t.Errorf("expected 3 values, got %d", len(slice))
		}
	})
}

func TestMerge_Composition(t *testing.T) {
	statusKey := state.NewKey[string]("status", "")
	countKey := state.NewKey[int]("count", 0)

	t.Run("merge two commands", func(t *testing.T) {
		cmd1 := New().With(SetValue(statusKey, "a"))
		cmd2 := New().With(SetValue(countKey, 42))

		merged := New().
			With(Merge(cmd1)).
			With(Merge(cmd2))

		updates, err := merged.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updates[statusKey.Name()] != "a" {
			t.Errorf("expected 'a', got %v", updates[statusKey.Name()])
		}
		if updates[countKey.Name()] != 42 {
			t.Errorf("expected 42, got %v", updates[countKey.Name()])
		}
	})

	t.Run("merge with error propagation", func(t *testing.T) {
		cmd1 := New()
		cmd1.err = fmt.Errorf("test error")

		merged := New().
			With(SetValue(statusKey, "ok")).
			With(Merge(cmd1))

		_, err := merged.Build()
		if err == nil {
			t.Error("expected error to be propagated")
		}
	})
}

func TestSetAll_MultipleCommands(t *testing.T) {
	statusKey := state.NewKey[string]("status", "")
	countKey := state.NewKey[int]("count", 0)
	msgKey := state.NewListKey[string]("messages", 0)

	cmd1 := New().With(SetValue(statusKey, "a"))
	cmd2 := New().With(SetValue(countKey, 1))
	cmd3 := New().With(Append(msgKey, "test"))

	merged := New().With(SetAll(cmd1, cmd2, cmd3))

	updates, err := merged.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 3 {
		t.Errorf("expected 3 keys, got %d", len(updates))
	}
	if updates[statusKey.Name()] != "a" {
		t.Errorf("expected 'a', got %v", updates[statusKey.Name()])
	}
	if updates[countKey.Name()] != 1 {
		t.Errorf("expected 1, got %v", updates[countKey.Name()])
	}
}

func TestCommand_Build(t *testing.T) {
	statusKey := state.NewKey[string]("status", "")
	countKey := state.NewKey[int]("count", 0)

	t.Run("empty command", func(t *testing.T) {
		updates, err := New().Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(updates) != 0 {
			t.Errorf("expected empty updates, got %d keys", len(updates))
		}
	})

	t.Run("with updates", func(t *testing.T) {
		updates, err := New().
			With(SetValue(statusKey, "done")).
			With(SetValue(countKey, 42)).
			Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(updates) != 2 {
			t.Errorf("expected 2 keys, got %d", len(updates))
		}
		if updates[statusKey.Name()] != "done" {
			t.Errorf("expected 'done', got %v", updates[statusKey.Name()])
		}
		if updates[countKey.Name()] != 42 {
			t.Errorf("expected 42, got %v", updates[countKey.Name()])
		}
	})

	t.Run("returns state.Updates type", func(t *testing.T) {
		updates, err := New().With(SetValue(statusKey, "test")).Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Verify it's the correct type
		var _ state.Updates = updates
	})
}

func TestCommand_To(t *testing.T) {
	statusKey := state.NewKey[string]("status", "")
	countKey := state.NewKey[int]("count", 0)

	t.Run("single target", func(t *testing.T) {
		targets, updates, err := New().
			With(SetValue(statusKey, "completed")).
			To("next")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 1 || targets[0] != "next" {
			t.Errorf("expected targets [next], got %v", targets)
		}
		if updates[statusKey.Name()] != "completed" {
			t.Errorf("expected 'completed', got %v", updates[statusKey.Name()])
		}
	})

	t.Run("multiple targets", func(t *testing.T) {
		targets, updates, err := New().
			With(SetValue(countKey, 42)).
			To("task1", "task2", "task3")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 3 {
			t.Errorf("expected 3 targets, got %d", len(targets))
		}
		if targets[0] != "task1" || targets[1] != "task2" || targets[2] != "task3" {
			t.Errorf("unexpected targets: %v", targets)
		}
		if updates[countKey.Name()] != 42 {
			t.Errorf("expected 42, got %v", updates[countKey.Name()])
		}
	})

	t.Run("end target", func(t *testing.T) {
		targets, _, err := New().
			With(SetValue(statusKey, "done")).
			To("__end__")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 1 || targets[0] != "__end__" {
			t.Errorf("expected targets [%s], got %v", "__end__", targets)
		}
	})

	t.Run("no updates", func(t *testing.T) {
		targets, updates, err := New().To("next")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 1 || targets[0] != "next" {
			t.Errorf("expected targets [next], got %v", targets)
		}
		if len(updates) != 0 {
			t.Errorf("expected empty updates, got %v", updates)
		}
	})
}

func TestCommand_ErrorHandling(t *testing.T) {
	statusKey := state.NewKey[string]("status", "")

	t.Run("error skips subsequent operations", func(t *testing.T) {
		cmd := New()
		errTest := fmt.Errorf("test error")
		cmd.err = errTest

		result := cmd.With(SetValue(statusKey, "should not be set"))

		if result != cmd {
			t.Error("With should return the same Command instance")
		}
		if len(cmd.m) != 0 {
			t.Error("With should not modify map when error present")
		}
	})

	t.Run("Build returns error", func(t *testing.T) {
		cmd := New()
		errTest := fmt.Errorf("test error")
		cmd.err = errTest

		updates, err := cmd.Build()

		if err == nil {
			t.Error("expected error from Build")
		}
		if updates != nil {
			t.Errorf("expected nil updates, got %v", updates)
		}
	})

	t.Run("To returns error", func(t *testing.T) {
		cmd := New()
		errTest := fmt.Errorf("test error")
		cmd.err = errTest

		targets, updates, err := cmd.To("next")

		if err == nil {
			t.Error("expected error from To")
		}
		if targets != nil {
			t.Errorf("expected nil targets, got %v", targets)
		}
		if updates != nil {
			t.Errorf("expected nil updates, got %v", updates)
		}
	})
}

func TestCommand_Chaining(t *testing.T) {
	statusKey := state.NewKey[string]("status", "")
	countKey := state.NewKey[int]("count", 0)
	tempKey := state.NewKey[float64]("temperature", 0.7)

	t.Run("long chain", func(t *testing.T) {
		targets, updates, err := New().
			With(SetValue(statusKey, "processing")).
			With(SetValue(countKey, 1)).
			With(SetValue(tempKey, 0.9)).
			To("processor", "validator")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 2 {
			t.Errorf("expected 2 targets, got %d", len(targets))
		}
		if len(updates) != 3 {
			t.Errorf("expected 3 updates, got %d", len(updates))
		}
	})
}

// Example showing typical usage in a node function
func ExampleCommand() {
	var messagesKey = state.NewListKey[string]("messages", 0)
	var countKey = state.NewKey[int]("count", 0)

	// Typical node function pattern
	nodeFunc := func(count int) ([]string, state.Updates, error) {
		// Use Command builder with type safety
		return New().
			With(Append(messagesKey, "processed")).
			With(SetValue(countKey, count+1)).
			To("next")
	}

	// Call the function
	targets, updates, err := nodeFunc(5)
	if err != nil {
		panic(err)
	}

	fmt.Println("Targets:", targets[0])
	fmt.Println("Count:", updates[countKey.Name()].(int))
	// Output:
	// Targets: next
	// Count: 6
}
