package sm

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// define a simple state enum for tests
type testState string

const (
	stateStart testState = "start"
	stateNext  testState = "next"
	stateEnd   testState = "end"
)

// simple frame
type frame struct {
	counter int
	flag    bool
}

func TestSingleTransition(t *testing.T) {
	m := New[testState, frame](stateStart)
	m.AddTransition(stateStart, stateEnd, nil)

	f := &frame{}
	visited := []testState{}

	err := m.Run(f, func(s testState, _ *frame) error {
		visited = append(visited, s)
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, []testState{stateStart, stateEnd}, visited)
}

func TestConditionalTransition(t *testing.T) {
	m := New[testState, frame](stateStart)
	m.AddTransition(stateStart, stateNext, func(f *frame) bool { return f.flag })
	m.AddTransition(stateStart, stateEnd, func(f *frame) bool { return !f.flag })

	// case 1: flag true
	f1 := &frame{flag: true}
	visited1 := []testState{}
	err := m.Run(f1, func(s testState, _ *frame) error {
		visited1 = append(visited1, s)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []testState{stateStart, stateNext}, visited1)

	// case 2: flag false
	f2 := &frame{flag: false}
	visited2 := []testState{}
	err = m.Run(f2, func(s testState, _ *frame) error {
		visited2 = append(visited2, s)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []testState{stateStart, stateEnd}, visited2)
}

func TestLoopingTransition(t *testing.T) {
	m := New[testState, frame](stateStart)
	m.AddTransition(stateStart, stateStart, func(f *frame) bool {
		f.counter++
		return f.counter < 3
	})
	m.AddTransition(stateStart, stateEnd, func(f *frame) bool {
		return f.counter >= 3
	})

	f := &frame{}
	visited := []testState{}

	err := m.Run(f, func(s testState, _ *frame) error {
		visited = append(visited, s)
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, []testState{stateStart, stateStart, stateStart, stateEnd}, visited)
	require.Equal(t, 3, f.counter)
}

func TestNoTransitionStops(t *testing.T) {
	m := New[testState, frame](stateStart)

	f := &frame{}
	visited := []testState{}
	err := m.Run(f, func(s testState, _ *frame) error {
		visited = append(visited, s)
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, []testState{stateStart}, visited)
}

func TestStepErrorStopsMachine(t *testing.T) {
	m := New[testState, frame](stateStart)
	m.AddTransition(stateStart, stateEnd, nil)

	f := &frame{}
	visited := []testState{}
	stepErr := errors.New("boom")

	err := m.Run(f, func(s testState, _ *frame) error {
		visited = append(visited, s)
		return stepErr
	})

	require.ErrorIs(t, err, stepErr)
	require.Equal(t, []testState{stateStart}, visited)
}
