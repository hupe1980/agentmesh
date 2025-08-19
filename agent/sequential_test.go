package agent

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// SequentialAgent Test Cases
func TestNewSequentialAgent(t *testing.T) {
	child1 := NewMockAgent("Child 1")
	child2 := NewMockAgent("Child 2")

	agent := NewSequentialAgent("Sequential Agent", child1, child2)

	assert.NotNil(t, agent)
	assert.Equal(t, "Sequential Agent", agent.Name())
	assert.Len(t, agent.children, 2)
	assert.Equal(t, child1, agent.children[0])
	assert.Equal(t, child2, agent.children[1])
}

func TestSequentialAgent_Run_Success(t *testing.T) {
	child1 := NewMockAgent("Child 1")
	child2 := NewMockAgent("Child 2")
	child3 := NewMockAgent("Child 3")

	agent := NewSequentialAgent("Sequential Agent", child1, child2, child3)

	ctx := context.Background()
	sessionID := "test-session"
	runID := "test-invocation"

	sess := core.NewSession(sessionID)
	agentInfo := core.AgentInfo{
		Name: "Sequential Agent",
		Type: "sequential",
	}

	userContent := core.Content{
		Role: "user",
		Parts: []core.Part{
			core.TextPart{Text: "test input"},
		},
	}

	emit := make(chan core.Event, 10)
	resume := make(chan struct{}, 1)

	invocationCtx := core.NewRunContext(
		ctx, sessionID, runID, agentInfo, userContent,
		emit, resume, sess, nil, nil, nil, logging.NoOpLogger{},
	)

	child1.On("Run", invocationCtx).Return(nil)
	child2.On("Run", invocationCtx).Return(nil)
	child3.On("Run", invocationCtx).Return(nil)

	err := agent.Run(invocationCtx)

	assert.NoError(t, err)
	child1.AssertExpectations(t)
	child2.AssertExpectations(t)
	child3.AssertExpectations(t)
}

func TestSequentialAgent_Run_FirstChildError(t *testing.T) {
	child1 := NewMockAgent("Child 1")
	child2 := NewMockAgent("Child 2")

	agent := NewSequentialAgent("Sequential Agent", child1, child2)

	ctx := context.Background()
	sess := core.NewSession("test-session")
	invocationCtx := core.NewRunContext(
		ctx, "test-session", "test-invocation",
		core.AgentInfo{Name: "Sequential Agent", Type: "sequential"},
		core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "test input"}}},
		make(chan core.Event, 10), make(chan struct{}, 1), sess,
		nil, nil, nil, logging.NoOpLogger{},
	)

	expectedErr := assert.AnError
	child1.On("Run", invocationCtx).Return(expectedErr)

	err := agent.Run(invocationCtx)

	assert.Error(t, err)
	assert.ErrorIs(t, err, expectedErr) // Check that the original error is wrapped
	child1.AssertExpectations(t)
	child2.AssertNotCalled(t, "Run")
}

func TestSequentialAgent_Run_NoChildren(t *testing.T) {
	agent := NewSequentialAgent("Sequential Agent")

	ctx := context.Background()
	sess := core.NewSession("test-session")
	invocationCtx := core.NewRunContext(
		ctx, "test-session", "test-invocation",
		core.AgentInfo{Name: "Sequential Agent", Type: "sequential"},
		core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "test input"}}},
		make(chan core.Event, 10), make(chan struct{}, 1), sess,
		nil, nil, nil, logging.NoOpLogger{},
	)

	err := agent.Run(invocationCtx)
	assert.NoError(t, err)
}

func TestSequentialAgent_ContextPropagation(t *testing.T) {
	child1 := NewMockAgent("Child 1")
	child2 := NewMockAgent("Child 2")

	agent := NewSequentialAgent("Sequential Agent", child1, child2)

	ctx := context.Background()
	sess := core.NewSession("test-session")
	invocationCtx := core.NewRunContext(
		ctx, "test-session", "test-invocation",
		core.AgentInfo{Name: "Sequential Agent", Type: "sequential"},
		core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "test input"}}},
		make(chan core.Event, 10), make(chan struct{}, 1), sess,
		nil, nil, nil, logging.NoOpLogger{},
	)

	child1.On("Run", mock.MatchedBy(func(ctx *core.RunContext) bool {
		return ctx == invocationCtx
	})).Return(nil)

	child2.On("Run", mock.MatchedBy(func(ctx *core.RunContext) bool {
		return ctx == invocationCtx
	})).Return(nil)

	err := agent.Run(invocationCtx)

	assert.NoError(t, err)
	child1.AssertExpectations(t)
	child2.AssertExpectations(t)
}
