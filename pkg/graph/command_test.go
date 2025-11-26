package graph

import (
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestCommand_Set(t *testing.T) {
	msgKey := state.NewKey[string]("messages", "")
	countKey := state.NewKey[int]("count", 0)

	tests := []struct {
		name     string
		setup    func() *Command
		validate func(*testing.T, *Command)
	}{
		{
			name: "single key",
			setup: func() *Command {
				return NewCommand().Set(msgKey, "hello")
			},
			validate: func(t *testing.T, cmd *Command) {
				if len(cmd.m) != 1 {
					t.Errorf("expected 1 key, got %d", len(cmd.m))
				}
				if cmd.m[msgKey.Name()] != "hello" {
					t.Errorf("expected 'hello', got %v", cmd.m[msgKey.Name()])
				}
			},
		},
		{
			name: "multiple keys",
			setup: func() *Command {
				return NewCommand().
					Set(msgKey, "hello").
					Set(countKey, 42)
			},
			validate: func(t *testing.T, cmd *Command) {
				if len(cmd.m) != 2 {
					t.Errorf("expected 2 keys, got %d", len(cmd.m))
				}
				if cmd.m[msgKey.Name()] != "hello" {
					t.Errorf("expected 'hello', got %v", cmd.m[msgKey.Name()])
				}
				if cmd.m[countKey.Name()] != 42 {
					t.Errorf("expected 42, got %v", cmd.m[countKey.Name()])
				}
			},
		},
		{
			name: "overwrite key",
			setup: func() *Command {
				return NewCommand().
					Set(msgKey, "first").
					Set(msgKey, "second")
			},
			validate: func(t *testing.T, cmd *Command) {
				if cmd.m[msgKey.Name()] != "second" {
					t.Errorf("expected 'second', got %v", cmd.m[msgKey.Name()])
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

func TestCommand_Build(t *testing.T) {
	msgKey := state.NewKey[string]("messages", "")
	countKey := state.NewKey[int]("count", 0)

	t.Run("empty command", func(t *testing.T) {
		updates, err := NewCommand().Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(updates) != 0 {
			t.Errorf("expected empty updates, got %d keys", len(updates))
		}
	})

	t.Run("with updates", func(t *testing.T) {
		updates, err := NewCommand().
			Set(msgKey, "hello").
			Set(countKey, 42).
			Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(updates) != 2 {
			t.Errorf("expected 2 keys, got %d", len(updates))
		}
		if updates[msgKey.Name()] != "hello" {
			t.Errorf("expected 'hello', got %v", updates[msgKey.Name()])
		}
		if updates[countKey.Name()] != 42 {
			t.Errorf("expected 42, got %v", updates[countKey.Name()])
		}
	})

	t.Run("returns state.Updates type", func(t *testing.T) {
		updates, err := NewCommand().Set(msgKey, "test").Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Verify it's the correct type
		var _ state.Updates = updates
	})
}

func TestCommand_To(t *testing.T) {
	msgKey := state.NewKey[string]("messages", "")
	countKey := state.NewKey[int]("count", 0)

	t.Run("single target", func(t *testing.T) {
		targets, updates, err := NewCommand().
			Set(msgKey, "hello").
			To("next")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 1 || targets[0] != "next" {
			t.Errorf("expected targets [next], got %v", targets)
		}
		if updates[msgKey.Name()] != "hello" {
			t.Errorf("expected 'hello', got %v", updates[msgKey.Name()])
		}
	})

	t.Run("multiple targets", func(t *testing.T) {
		targets, updates, err := NewCommand().
			Set(countKey, 42).
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

	t.Run("EndNode target", func(t *testing.T) {
		targets, _, err := NewCommand().
			Set(msgKey, "done").
			To(EndNode)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 1 || targets[0] != EndNode {
			t.Errorf("expected targets [%s], got %v", EndNode, targets)
		}
	})

	t.Run("no updates", func(t *testing.T) {
		targets, updates, err := NewCommand().To("next")

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
	msgKey := state.NewKey[string]("messages", "")

	t.Run("error skips subsequent Set calls", func(t *testing.T) {
		cmd := NewCommand()
		errTest := fmt.Errorf("test error")
		cmd.err = errTest

		result := cmd.Set(msgKey, "should not be set")

		if result != cmd {
			t.Error("Set should return the same Command instance")
		}
		if len(cmd.m) != 0 {
			t.Error("Set should not modify map when error present")
		}
	})

	t.Run("Build returns error", func(t *testing.T) {
		cmd := NewCommand()
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
		cmd := NewCommand()
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
	msgKey := state.NewKey[string]("messages", "")
	countKey := state.NewKey[int]("count", 0)
	tempKey := state.NewKey[float64]("temperature", 0.7)

	t.Run("long chain", func(t *testing.T) {
		targets, updates, err := NewCommand().
			Set(msgKey, "hello").
			Set(countKey, 1).
			Set(tempKey, 0.9).
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
	var messagesKey = state.NewKey[[]string]("messages", nil)
	var countKey = state.NewKey[int]("count", 0)

	// Typical node function pattern
	nodeFunc := func(msgs []string, count int) ([]string, state.Updates, error) {
		// Process data...
		newMsg := "processed"

		// Use Command builder for clean syntax
		return NewCommand().
			Set(messagesKey, append(msgs, newMsg)).
			Set(countKey, count+1).
			To("next")
	}

	// Call the function
	targets, updates, err := nodeFunc([]string{"msg1"}, 5)
	if err != nil {
		panic(err)
	}

	fmt.Println("Targets:", targets[0])
	fmt.Println("Count:", updates[countKey.Name()].(int))
	// Output:
	// Targets: next
	// Count: 6
}
