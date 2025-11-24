package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestResumeValueContext verifies resume values context handling
func TestResumeValueContext(t *testing.T) {
	ctx := context.Background()

	// Context without resume value should return nil
	resumeVal := ResumeValueFromContext(ctx)
	assert.Nil(t, resumeVal)

	// Context with resume value should return the value
	expectedValue := map[string]any{"decision": "approve"}
	ctxWithValue := context.WithValue(ctx, resumeValueKey, expectedValue)
	resumeVal = ResumeValueFromContext(ctxWithValue)
	assert.Equal(t, expectedValue, resumeVal)
}
