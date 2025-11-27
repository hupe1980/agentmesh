package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name: "error with node",
			err: ValidationError{
				Type:    ErrorTypeCycle,
				Node:    "nodeA",
				Message: "creates a cycle",
			},
			expected: `[CYCLE] node="nodeA": creates a cycle`,
		},
		{
			name: "error without node",
			err: ValidationError{
				Type:    ErrorTypeDisconnected,
				Node:    "",
				Message: "graph has disconnected components",
			},
			expected: "[DISCONNECTED] graph has disconnected components",
		},
		{
			name: "missing node error",
			err: ValidationError{
				Type:    ErrorTypeMissingNode,
				Node:    "missing",
				Message: "node does not exist",
			},
			expected: `[MISSING_NODE] node="missing": node does not exist`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestDefaultValidationOptions(t *testing.T) {
	opts := DefaultValidationOptions()

	assert.Equal(t, ValidationLevelBasic, opts.Level)
	assert.False(t, opts.SkipValidation)
	assert.True(t, opts.AllowCycles)
	assert.False(t, opts.AllowDisconnectedNodes)
}

func TestStrictValidationOptions(t *testing.T) {
	opts := StrictValidationOptions()

	assert.Equal(t, ValidationLevelStrict, opts.Level)
	assert.False(t, opts.SkipValidation)
	assert.False(t, opts.AllowCycles)
	assert.False(t, opts.AllowDisconnectedNodes)
}

func TestValidationErrorTypes(t *testing.T) {
	// Test that all error type constants are defined correctly
	assert.Equal(t, ValidationErrorType("CYCLE"), ErrorTypeCycle)
	assert.Equal(t, ValidationErrorType("DISCONNECTED"), ErrorTypeDisconnected)
	assert.Equal(t, ValidationErrorType("INVALID_ENTRY_NODE"), ErrorTypeInvalidEntryNode)
	assert.Equal(t, ValidationErrorType("INVALID_END_NODE"), ErrorTypeInvalidEndNode)
	assert.Equal(t, ValidationErrorType("MISSING_NODE"), ErrorTypeMissingNode)
	assert.Equal(t, ValidationErrorType("INVALID_BRANCH"), ErrorTypeInvalidBranch)
	assert.Equal(t, ValidationErrorType("INVALID_EDGE"), ErrorTypeInvalidEdge)
	assert.Equal(t, ValidationErrorType("DUPLICATE_NODE"), ErrorTypeDuplicateNode)
}

func TestValidationLevel(t *testing.T) {
	// Test validation levels are distinct
	assert.NotEqual(t, ValidationLevelNone, ValidationLevelBasic)
	assert.NotEqual(t, ValidationLevelBasic, ValidationLevelStrict)
	assert.NotEqual(t, ValidationLevelNone, ValidationLevelStrict)

	// Test ordering
	assert.Less(t, ValidationLevelNone, ValidationLevelBasic)
	assert.Less(t, ValidationLevelBasic, ValidationLevelStrict)
}
