package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientWrapper_NilClient(t *testing.T) {
	_, err := NewClientWrapper(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client cannot be nil")
}

func TestNewModelFromClientWrapper_NilWrapper(t *testing.T) {
	_, err := NewModelFromClientWrapper(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrapper cannot be nil")
}

func TestNewModelFromClient_NilClient(t *testing.T) {
	_, err := NewModelFromClient(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client cannot be nil")
}
