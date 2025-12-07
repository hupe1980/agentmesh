package viz

import (
	"context"
	"sync"
)

const (
	// executionCommandQueueSize is the buffer size for execution control commands.
	// A small buffer (10) is sufficient since commands are control signals
	// (pause, resume, step) that are infrequent and should be processed quickly.
	executionCommandQueueSize = 10
)

// ExecutionCommand represents a control command for graph execution
type ExecutionCommand string

// Execution command constants
const (
	CommandPause    ExecutionCommand = "pause"
	CommandResume   ExecutionCommand = "resume"
	CommandStep     ExecutionCommand = "step"      // Step forward one superstep
	CommandStepNode ExecutionCommand = "step_node" // Step to next node execution
	CommandStop     ExecutionCommand = "stop"
	CommandContinue ExecutionCommand = "continue" // Continue to next breakpoint
	CommandJumpTo   ExecutionCommand = "jump_to"  // Jump to specific superstep (time-travel)
)

// ExecutionState represents the current execution state
type ExecutionState string

// Execution state constants
const (
	StateRunning      ExecutionState = "running"
	StatePaused       ExecutionState = "paused"
	StateSteppingNode ExecutionState = "stepping_node"
	StateSteppingStep ExecutionState = "stepping_step"
	StateStopped      ExecutionState = "stopped"
	StateCompleted    ExecutionState = "completed"
	StateError        ExecutionState = "error"
)

// Breakpoint represents a conditional or unconditional breakpoint
type Breakpoint struct {
	ID        string                 `json:"id"`
	Type      BreakpointType         `json:"type"`
	Enabled   bool                   `json:"enabled"`
	Node      string                 `json:"node,omitempty"`      // For node breakpoints
	Superstep int64                  `json:"superstep,omitempty"` // For superstep breakpoints
	Condition string                 `json:"condition,omitempty"` // JavaScript expression for conditional
	HitCount  int                    `json:"hit_count"`           // Number of times hit
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// BreakpointType defines the type of breakpoint
type BreakpointType string

// Breakpoint type constants
const (
	BreakpointNode      BreakpointType = "node"      // Break on specific node
	BreakpointSuperstep BreakpointType = "superstep" // Break on specific superstep
	BreakpointCondition BreakpointType = "condition" // Break on condition
	BreakpointError     BreakpointType = "error"     // Break on any error
)

// ExecutionController manages execution control for debugging
type ExecutionController struct {
	mu           sync.RWMutex
	runID        string
	state        ExecutionState
	breakpoints  map[string]*Breakpoint
	commandQueue chan ExecutionCommand
	currentNode  string
	currentStep  int64
	stepMode     bool  // True when stepping
	stepTarget   int64 // Target superstep when jumping
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewExecutionController creates a new execution controller
func NewExecutionController(ctx context.Context, runID string) *ExecutionController {
	ctxWithCancel, cancel := context.WithCancel(ctx)

	return &ExecutionController{
		runID:        runID,
		state:        StateRunning,
		breakpoints:  make(map[string]*Breakpoint),
		commandQueue: make(chan ExecutionCommand, executionCommandQueueSize),
		ctx:          ctxWithCancel,
		cancel:       cancel,
	}
}

// GetState returns the current execution state
func (ec *ExecutionController) GetState() ExecutionState {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.state
}

// SetState updates the execution state
func (ec *ExecutionController) SetState(state ExecutionState) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.state = state
}

// SendCommand sends an execution command
func (ec *ExecutionController) SendCommand(cmd ExecutionCommand) error {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	// Validate command based on current state
	if ec.state == StateStopped || ec.state == StateCompleted {
		return &InvalidCommandError{Command: cmd, State: ec.state}
	}

	select {
	case ec.commandQueue <- cmd:
		return nil
	default:
		return ErrCommandQueueFull
	}
}

// WaitForCommand blocks until a command is received or context is cancelled
func (ec *ExecutionController) WaitForCommand() (ExecutionCommand, error) {
	select {
	case cmd := <-ec.commandQueue:
		return cmd, nil
	case <-ec.ctx.Done():
		return "", ec.ctx.Err()
	}
}

// AddBreakpoint adds a new breakpoint
func (ec *ExecutionController) AddBreakpoint(bp *Breakpoint) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if bp.ID == "" {
		bp.ID = generateEventID() // Reuse ID generator
	}
	bp.Enabled = true
	bp.HitCount = 0

	ec.breakpoints[bp.ID] = bp
}

// RemoveBreakpoint removes a breakpoint
func (ec *ExecutionController) RemoveBreakpoint(id string) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if _, exists := ec.breakpoints[id]; !exists {
		return &BreakpointNotFoundError{ID: id}
	}

	delete(ec.breakpoints, id)
	return nil
}

// EnableBreakpoint enables a breakpoint
func (ec *ExecutionController) EnableBreakpoint(id string, enabled bool) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	bp, exists := ec.breakpoints[id]
	if !exists {
		return &BreakpointNotFoundError{ID: id}
	}

	bp.Enabled = enabled
	return nil
}

// GetBreakpoints returns all breakpoints
func (ec *ExecutionController) GetBreakpoints() []*Breakpoint {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	breakpoints := make([]*Breakpoint, 0, len(ec.breakpoints))
	for _, bp := range ec.breakpoints {
		// Create a copy to avoid race conditions
		bpCopy := *bp
		breakpoints = append(breakpoints, &bpCopy)
	}

	return breakpoints
}

// CheckBreakpoint checks if execution should break at current point
func (ec *ExecutionController) CheckBreakpoint(node string, superstep int64, hasError bool) bool {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.currentNode = node
	ec.currentStep = superstep

	for _, bp := range ec.breakpoints {
		if !bp.Enabled {
			continue
		}

		shouldBreak := false

		switch bp.Type {
		case BreakpointNode:
			if bp.Node == node {
				shouldBreak = true
			}
		case BreakpointSuperstep:
			if bp.Superstep == superstep {
				shouldBreak = true
			}
		case BreakpointError:
			if hasError {
				shouldBreak = true
			}
		case BreakpointCondition:
			// Condition evaluation would happen here
			// For now, we don't evaluate JavaScript conditions
			shouldBreak = false
		}

		if shouldBreak {
			bp.HitCount++
			ec.state = StatePaused
			return true
		}
	}

	return false
}

// ShouldPause checks if execution should pause (for stepping)
func (ec *ExecutionController) ShouldPause(node string, superstep int64) bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	// Check if we're in step mode
	if ec.stepMode {
		if superstep > ec.currentStep {
			return true
		}
	}

	// Check if we've reached step target (for jump_to command)
	if ec.stepTarget > 0 && superstep >= ec.stepTarget {
		return true
	}

	return false
}

// SetStepMode enables or disables step mode
func (ec *ExecutionController) SetStepMode(enabled bool, targetStep int64) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.stepMode = enabled
	ec.stepTarget = targetStep

	if enabled {
		ec.state = StateSteppingStep
	}
}

// GetCurrentPosition returns the current execution position
func (ec *ExecutionController) GetCurrentPosition() (node string, superstep int64) {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.currentNode, ec.currentStep
}

// Stop stops the execution controller
func (ec *ExecutionController) Stop() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.state = StateStopped
	ec.cancel()
	close(ec.commandQueue)
}

// Context returns the controller's context
func (ec *ExecutionController) Context() context.Context {
	return ec.ctx
}
