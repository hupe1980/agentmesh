package tool

import "strings"

const longRunningInstruction = `

NOTE: This is a long-running operation.
Do not call this tool again if it has already returned some intermediate or pending status.`

// LongRunningTool wraps a FuncTool and annotates its metadata as long-running.
type LongRunningTool[T any] struct {
	*FuncTool[T]
}

// NewLongRunningTool creates a FuncTool variant that warns callers about long execution times.
func NewLongRunningTool[T any](
	name, description string,
	parameters map[string]any,
	fn Func[T],
) *LongRunningTool[T] {
	return &LongRunningTool[T]{
		FuncTool: NewFuncTool(name, description, parameters, fn),
	}
}

// Description returns the base description plus a long-running instruction hint.
func (t *LongRunningTool[T]) Description() string {
	desc := t.FuncTool.Description()
	if desc == "" {
		return strings.TrimPrefix(longRunningInstruction, "\n\n")
	}

	return desc + longRunningInstruction
}

// IsLongRunning indicates that this tool may take a long time to complete.
func (t *LongRunningTool[T]) IsLongRunning() bool {
	return true
}
