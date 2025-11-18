package checkpoint

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignCheckpoint(t *testing.T) {
	signingKey := []byte("test-signing-key-at-least-32-bytes-long")

	cp := &Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		State: map[string]any{
			"counter": 42,
			"status":  "running",
		},
		CompletedNodes: []string{"node1", "node2"},
		PausedNodes:    []string{"node3"},
	}

	t.Run("successful signing", func(t *testing.T) {
		signature, err := SignCheckpoint(cp, signingKey)
		require.NoError(t, err)
		assert.NotNil(t, signature)
		assert.Len(t, signature, 32) // HMAC-SHA256 produces 32 bytes
	})

	t.Run("empty signing key error", func(t *testing.T) {
		_, err := SignCheckpoint(cp, []byte{})
		assert.ErrorIs(t, err, ErrEmptySigningKey)
	})

	t.Run("nil signing key error", func(t *testing.T) {
		_, err := SignCheckpoint(cp, nil)
		assert.ErrorIs(t, err, ErrSigningKeyRequired)
	})

	t.Run("nil checkpoint error", func(t *testing.T) {
		_, err := SignCheckpoint(nil, signingKey)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "checkpoint is nil")
	})

	t.Run("deterministic signatures", func(t *testing.T) {
		// Same checkpoint should produce same signature
		sig1, err1 := SignCheckpoint(cp, signingKey)
		require.NoError(t, err1)

		sig2, err2 := SignCheckpoint(cp, signingKey)
		require.NoError(t, err2)

		assert.Equal(t, sig1, sig2)
	})

	t.Run("different keys produce different signatures", func(t *testing.T) {
		key1 := []byte("key1-at-least-32-bytes-long-for-security")
		key2 := []byte("key2-at-least-32-bytes-long-for-security")

		sig1, err1 := SignCheckpoint(cp, key1)
		require.NoError(t, err1)

		sig2, err2 := SignCheckpoint(cp, key2)
		require.NoError(t, err2)

		assert.NotEqual(t, sig1, sig2)
	})
}

func TestVerifyCheckpoint(t *testing.T) {
	signingKey := []byte("test-signing-key-at-least-32-bytes-long")

	cp := &Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Version:   1,
		Timestamp: time.Now(),
		State: map[string]any{
			"counter": 42,
			"status":  "running",
		},
		CompletedNodes: []string{"node1", "node2"},
		PausedNodes:    []string{"node3"},
	}

	t.Run("successful verification", func(t *testing.T) {
		signature, err := SignCheckpoint(cp, signingKey)
		require.NoError(t, err)

		cp.Signature = signature

		err = VerifyCheckpoint(cp, signingKey)
		assert.NoError(t, err)
	})

	t.Run("invalid signature detection", func(t *testing.T) {
		signature, err := SignCheckpoint(cp, signingKey)
		require.NoError(t, err)

		cp.Signature = signature

		// Tamper with the checkpoint
		cp.State["counter"] = 999

		err = VerifyCheckpoint(cp, signingKey)
		assert.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("wrong key detection", func(t *testing.T) {
		signature, err := SignCheckpoint(cp, signingKey)
		require.NoError(t, err)

		cp.Signature = signature

		wrongKey := []byte("wrong-signing-key-at-least-32-bytes-long")
		err = VerifyCheckpoint(cp, wrongKey)
		assert.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("empty signing key error", func(t *testing.T) {
		err := VerifyCheckpoint(cp, []byte{})
		assert.ErrorIs(t, err, ErrEmptySigningKey)
	})

	t.Run("nil signing key error", func(t *testing.T) {
		err := VerifyCheckpoint(cp, nil)
		assert.ErrorIs(t, err, ErrSigningKeyRequired)
	})

	t.Run("nil checkpoint error", func(t *testing.T) {
		err := VerifyCheckpoint(nil, signingKey)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "checkpoint is nil")
	})
}

func TestInMemoryCheckpointerWithSigning(t *testing.T) {
	ctx := context.Background()
	signingKey := []byte("test-signing-key-at-least-32-bytes-long")

	t.Run("save and load with signing", func(t *testing.T) {
		checkpointer := NewInMemoryCheckpointer(WithSigning(signingKey))

		cp := &Checkpoint{
			RunID:     "test-run",
			Superstep: 1,
			Version:   1,
			Timestamp: time.Now(),
			State: map[string]any{
				"counter": 42,
			},
			CompletedNodes: []string{},
			PausedNodes:    []string{},
		}

		// Save checkpoint (should sign automatically)
		err := checkpointer.Save(ctx, cp)
		require.NoError(t, err)

		// Load checkpoint (should verify automatically)
		loaded, err := checkpointer.Load(ctx, "test-run")
		require.NoError(t, err)
		require.NotNil(t, loaded)

		assert.Equal(t, cp.RunID, loaded.RunID)
		assert.Equal(t, cp.Superstep, loaded.Superstep)
		assert.Equal(t, cp.State["counter"], loaded.State["counter"])
		assert.NotNil(t, loaded.Signature)
		assert.Len(t, loaded.Signature, 32)
	})

	t.Run("tampering detection on load", func(t *testing.T) {
		checkpointer := NewInMemoryCheckpointer(WithSigning(signingKey))

		cp := &Checkpoint{
			RunID:     "tamper-test",
			Superstep: 1,
			Version:   1,
			Timestamp: time.Now(),
			State: map[string]any{
				"counter": 42,
			},
		}

		// Save checkpoint
		err := checkpointer.Save(ctx, cp)
		require.NoError(t, err)

		// Directly tamper with stored checkpoint
		checkpointer.mu.Lock()
		checkpointer.checkpoints["tamper-test"][0].State["counter"] = 999
		checkpointer.mu.Unlock()

		// Load should fail verification
		_, err = checkpointer.Load(ctx, "tamper-test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature verification failed")
	})

	t.Run("load at superstep with signing", func(t *testing.T) {
		checkpointer := NewInMemoryCheckpointer(WithSigning(signingKey))

		// Save multiple checkpoints
		for i := int64(1); i <= 3; i++ {
			cp := &Checkpoint{
				RunID:     "multi-step",
				Superstep: i,
				Version:   1,
				Timestamp: time.Now(),
				State: map[string]any{
					"step": i,
				},
			}
			err := checkpointer.Save(ctx, cp)
			require.NoError(t, err)
		}

		// Load specific superstep
		loaded, err := checkpointer.LoadAtSuperstep(ctx, "multi-step", 2)
		require.NoError(t, err)
		require.NotNil(t, loaded)

		assert.Equal(t, int64(2), loaded.Superstep)
		assert.Equal(t, int64(2), loaded.State["step"])
		assert.NotNil(t, loaded.Signature)
	})

	t.Run("list with signing", func(t *testing.T) {
		checkpointer := NewInMemoryCheckpointer(WithSigning(signingKey))

		// Save multiple checkpoints
		for i := int64(1); i <= 3; i++ {
			cp := &Checkpoint{
				RunID:     "list-test",
				Superstep: i,
				Version:   1,
				Timestamp: time.Now(),
				State: map[string]any{
					"step": i,
				},
			}
			err := checkpointer.Save(ctx, cp)
			require.NoError(t, err)
		}

		// List all checkpoints
		checkpoints, err := checkpointer.List(ctx, "list-test")
		require.NoError(t, err)
		assert.Len(t, checkpoints, 3)

		// All should have signatures
		for _, cp := range checkpoints {
			assert.NotNil(t, cp.Signature)
			assert.Len(t, cp.Signature, 32)
		}
	})

	t.Run("without signing option", func(t *testing.T) {
		// Checkpointer without signing
		checkpointer := NewInMemoryCheckpointer()

		cp := &Checkpoint{
			RunID:     "unsigned",
			Superstep: 1,
			Version:   1,
			Timestamp: time.Now(),
			State: map[string]any{
				"counter": 42,
			},
		}

		// Save checkpoint (no signing)
		err := checkpointer.Save(ctx, cp)
		require.NoError(t, err)

		// Load checkpoint (no verification)
		loaded, err := checkpointer.Load(ctx, "unsigned")
		require.NoError(t, err)
		require.NotNil(t, loaded)

		// Signature should be nil when signing is not enabled
		assert.Nil(t, loaded.Signature)
	})

	t.Run("tampering detection on list", func(t *testing.T) {
		checkpointer := NewInMemoryCheckpointer(WithSigning(signingKey))

		// Save checkpoint
		cp := &Checkpoint{
			RunID:     "list-tamper",
			Superstep: 1,
			Version:   1,
			Timestamp: time.Now(),
			State: map[string]any{
				"counter": 42,
			},
		}
		err := checkpointer.Save(ctx, cp)
		require.NoError(t, err)

		// Tamper with stored checkpoint
		checkpointer.mu.Lock()
		checkpointer.checkpoints["list-tamper"][0].State["counter"] = 999
		checkpointer.mu.Unlock()

		// List should fail verification
		_, err = checkpointer.List(ctx, "list-tamper")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature verification failed")
	})
}
