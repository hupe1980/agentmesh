package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorFormatting(t *testing.T) {
	err := NewError("demo", "something failed", "E123")
	assert.Contains(t, err.Error(), "E123")
	assert.Contains(t, err.Error(), "demo")
}
