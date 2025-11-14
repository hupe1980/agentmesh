package graph

import (
	stateif "github.com/hupe1980/agentmesh/pkg/state"
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/stretchr/testify/require"
)

func TestBuilder_BasicUsage(t *testing.T) {
	t.Parallel()

	builder, err := NewBuilder()
	require.NoError(t, err)

	compiled, err := builder.
		WithMaxMessages(0).
		WithInitialChannels(func(state *ChannelState) {
			state.Set("count", 0)
		}).
		Node("increment", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			count, _ := s.Get("count").(int)
			return &NodeResult{
				Updates: map[string]any{"count": count + 1},
			}, nil
		}).
		AddEdge(StartNode, "increment").
		AddEdge("increment", EndNode).
		Compile()

	require.NoError(t, err)
	require.NotNil(t, compiled)

	_, err = Last(compiled.Run(context.Background(), nil))
	require.NoError(t, err)

	count := compiled.State().Get("count")
	require.Equal(t, 1, count)
}

func TestBuilder_WithMaxMessagesPreservesExistingChannels(t *testing.T) {
	t.Parallel()

	builder, err := NewBuilder()
	require.NoError(t, err)

	builder.
		WithInitialChannels(func(state *ChannelState) {
			state.Set("status", "pending")
		}).
		WithMaxMessages(5)

	builder.Node("noop", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return nil, nil
	})

	builder.StartTo("noop")
	builder.ToEnd("noop")

	compiled, err := builder.Compile()
	require.NoError(t, err)

	_, err = Last(compiled.Run(context.Background(), nil))
	require.NoError(t, err)

	stateManager := compiled.State()
	status := stateManager.Get("status")
	require.Equal(t, "pending", status)

	graphState, ok := stateManager.(*ChannelState)
	require.True(t, ok)

	ch, ok := graphState.GetChannel("messages")
	require.True(t, ok)
	topic, ok := ch.(*channel.TopicChannel)
	require.True(t, ok)
	require.Equal(t, 5, topic.MaxValues())
}

func TestBuilder_Chain(t *testing.T) {
	t.Parallel()

	builder, err := NewBuilder()
	require.NoError(t, err)

	builder.WithMaxMessages(0).WithInitialChannels(func(state *ChannelState) {
		state.Set("value", 1)
	})

	builder.Node("double", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		val, _ := s.Get("value").(int)
		return &NodeResult{Updates: map[string]any{"value": val * 2}}, nil
	})

	builder.Node("add_ten", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		val, _ := s.Get("value").(int)
		return &NodeResult{Updates: map[string]any{"value": val + 10}}, nil
	})

	builder.Node("square", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		val, _ := s.Get("value").(int)
		return &NodeResult{Updates: map[string]any{"value": val * val}}, nil
	})

	compiled := builder.Chain("double", "add_ten", "square").MustCompile()

	_, err = Last(compiled.Run(context.Background(), nil))
	require.NoError(t, err)

	result := compiled.State().Get("value")
	require.Equal(t, 144, result)
}

func TestBuilder_Parallel(t *testing.T) {
	t.Parallel()

	appendReducer := func(oldValue, newValue any) any {
		oldSlice, _ := oldValue.([]any)
		newSlice, _ := newValue.([]any)
		return append(oldSlice, newSlice...)
	}

	builder, err := NewBuilder()
	require.NoError(t, err)

	builder.
		WithMaxMessages(0).
		WithInitialChannels(func(state *ChannelState) {
			state.AddChannel(channel.NewBinaryOpChannel("results", []any{}, appendReducer))
		})

	builder.Node("start", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{Updates: map[string]any{"triggered": true}}, nil
	})

	builder.Node("task_a", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{Updates: map[string]any{"results": []any{"A"}}}, nil
	})

	builder.Node("task_b", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{Updates: map[string]any{"results": []any{"B"}}}, nil
	})

	builder.Node("task_c", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{Updates: map[string]any{"results": []any{"C"}}}, nil
	})

	builder.Node("aggregate", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{Updates: map[string]any{"done": true}}, nil
	})

	builder.AddEdge(StartNode, "start")
	builder.Parallel("start", []string{"task_a", "task_b", "task_c"}, "aggregate")
	builder.ToEnd("aggregate")

	compiled, err := builder.Compile()
	require.NoError(t, err)

	_, err = Last(compiled.Run(context.Background(), nil))
	require.NoError(t, err)

	results, ok := compiled.State().Get("results").([]any)
	require.True(t, ok)
	require.Len(t, results, 3)
	require.Contains(t, results, "A")
	require.Contains(t, results, "B")
	require.Contains(t, results, "C")
}

func TestBuilder_ConditionalRoute(t *testing.T) {
	t.Parallel()

	builder, err := NewBuilder()
	require.NoError(t, err)

	builder.WithMaxMessages(0).WithInitialChannels(func(state *ChannelState) {
		state.Set("score", 75)
	})

	builder.Node("evaluate", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{Updates: map[string]any{"evaluated": true}}, nil
	})

	builder.Node("pass", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{Updates: map[string]any{"result": "passed"}}, nil
	})

	builder.Node("fail", func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
		return &NodeResult{Updates: map[string]any{"result": "failed"}}, nil
	})

	builder.StartTo("evaluate")
	builder.ConditionalRoute("evaluate", func(ctx context.Context, s stateif.Reader) (string, error) {
		score, _ := s.Get("score").(int)
		if score >= 70 {
			return "pass", nil
		}
		return "fail", nil
	}, []string{"pass", "fail"})
	builder.ToEnd("pass")
	builder.ToEnd("fail")

	compiled := builder.MustCompile()

	_, err = Last(compiled.Run(context.Background(), nil))
	require.NoError(t, err)

	result := compiled.State().Get("result")
	require.Equal(t, "passed", result)
}
