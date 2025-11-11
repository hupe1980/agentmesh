package checkpoint_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptedCheckpointer(t *testing.T) {
	ctx := context.Background()

	// Create base in-memory checkpointer
	base := checkpoint.NewInMemoryCheckpointer()

	// Create encryption key (32 bytes for AES-256)
	key := []byte("12345678901234567890123456789012")

	// Create encryptor
	encryptor, err := checkpoint.NewAES256GCMEncryptor(key)
	require.NoError(t, err)

	// Create encrypted checkpointer
	encrypted, err := checkpoint.NewEncryptedCheckpointer(base, encryptor)
	require.NoError(t, err)

	// Create test checkpoint
	cp := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Metadata: map[string]any{
			"created_at": "2025-11-11T00:00:00Z",
		},
		State: map[string]any{
			"counter": 42,
			"message": "sensitive data",
		},
	}

	// Save encrypted
	err = encrypted.Save(ctx, cp)
	require.NoError(t, err)

	// Load raw from base (should be encrypted)
	rawCP, err := base.Load(ctx, "test-run")
	require.NoError(t, err)
	assert.True(t, rawCP.Metadata["encrypted"].(bool))
	assert.Equal(t, "aes-256-gcm", rawCP.Metadata["algorithm"])
	assert.NotNil(t, rawCP.Metadata["payload"])

	// Load decrypted
	loadedCP, err := encrypted.Load(ctx, "test-run")
	require.NoError(t, err)
	assert.Equal(t, "test-run", loadedCP.RunID)
	assert.Equal(t, int64(1), loadedCP.Superstep)
	assert.Equal(t, float64(42), loadedCP.State["counter"]) // JSON unmarshaling converts to float64
	assert.Equal(t, "sensitive data", loadedCP.State["message"])
}

func TestEncryptedCheckpointer_WrongKeySize(t *testing.T) {
	base := checkpoint.NewInMemoryCheckpointer()

	// Try with wrong key size
	_, err := checkpoint.NewAES256GCMEncryptor([]byte("short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")

	// Verify nil encryptor error
	_, err = checkpoint.NewEncryptedCheckpointer(base, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encryptor is required")
}

func TestEncryptedCheckpointer_BackwardsCompatibility(t *testing.T) {
	ctx := context.Background()

	base := checkpoint.NewInMemoryCheckpointer()
	key := []byte("12345678901234567890123456789012")
	encryptor, err := checkpoint.NewAES256GCMEncryptor(key)
	require.NoError(t, err)
	encrypted, err := checkpoint.NewEncryptedCheckpointer(base, encryptor)
	require.NoError(t, err)

	// Save unencrypted checkpoint directly to base
	unencryptedCP := &checkpoint.Checkpoint{
		RunID:     "old-run",
		Superstep: 5,
		State: map[string]any{
			"data": "unencrypted",
		},
	}
	err = base.Save(ctx, unencryptedCP)
	require.NoError(t, err)

	// Load through encrypted checkpointer (should still work)
	loadedCP, err := encrypted.Load(ctx, "old-run")
	require.NoError(t, err)
	assert.Equal(t, "old-run", loadedCP.RunID)
	assert.Equal(t, "unencrypted", loadedCP.State["data"])
}

func TestMultiKeyCheckpointer(t *testing.T) {
	ctx := context.Background()

	base := checkpoint.NewInMemoryCheckpointer()
	oldKey := []byte("old12345old12345old12345old12345")
	newKey := []byte("new12345new12345new12345new12345")

	// Create checkpointer with old key
	oldEncryptor, err := checkpoint.NewAES256GCMEncryptor(oldKey)
	require.NoError(t, err)
	oldEncrypted, err := checkpoint.NewEncryptedCheckpointer(base, oldEncryptor)
	require.NoError(t, err)

	// Save with old key
	cp := &checkpoint.Checkpoint{
		RunID:     "rotation-test",
		Superstep: 1,
		State: map[string]any{
			"data": "secret",
		},
	}
	err = oldEncrypted.Save(ctx, cp)
	require.NoError(t, err)

	// Create multi-key checkpointer with new key as current, old key as fallback
	multiKey, err := checkpoint.NewMultiKeyCheckpointer(base, newKey, oldKey)
	require.NoError(t, err)

	// Load should work with old key
	loadedCP, err := multiKey.Load(ctx, "rotation-test")
	require.NoError(t, err)
	assert.Equal(t, "rotation-test", loadedCP.RunID)
	assert.Equal(t, "secret", loadedCP.State["data"])

	// Save again (will use new key)
	cp.Superstep = 2
	err = multiKey.Save(ctx, cp)
	require.NoError(t, err)

	// Should still be loadable
	loadedCP, err = multiKey.Load(ctx, "rotation-test")
	require.NoError(t, err)
	assert.Equal(t, int64(2), loadedCP.Superstep)
}

func TestDeriveKeyFromPassword(t *testing.T) {
	password := "my-secure-password"
	salt := []byte("unique-salt-value")

	// Derive key
	key1 := checkpoint.DeriveKeyFromPassword(password, salt)
	assert.Equal(t, 32, len(key1))

	// Same password and salt should produce same key
	key2 := checkpoint.DeriveKeyFromPassword(password, salt)
	assert.Equal(t, key1, key2)

	// Different password should produce different key
	key3 := checkpoint.DeriveKeyFromPassword("different-password", salt)
	assert.NotEqual(t, key1, key3)

	// Different salt should produce different key
	key4 := checkpoint.DeriveKeyFromPassword(password, []byte("different-salt"))
	assert.NotEqual(t, key1, key4)
}

func TestEncryptedCheckpointer_LoadAtSuperstep(t *testing.T) {
	ctx := context.Background()

	base := checkpoint.NewInMemoryCheckpointer()
	key := []byte("12345678901234567890123456789012")
	encryptor, err := checkpoint.NewAES256GCMEncryptor(key)
	require.NoError(t, err)
	encrypted, err := checkpoint.NewEncryptedCheckpointer(base, encryptor)
	require.NoError(t, err)

	// Save multiple supersteps
	for i := int64(0); i < 5; i++ {
		cp := &checkpoint.Checkpoint{
			RunID:     "multi-step",
			Superstep: i,
			State: map[string]any{
				"step": i,
			},
		}
		err = encrypted.Save(ctx, cp)
		require.NoError(t, err)
	}

	// Load specific superstep
	cp, err := encrypted.LoadAtSuperstep(ctx, "multi-step", 2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), cp.Superstep)
	assert.Equal(t, float64(2), cp.State["step"]) // JSON unmarshaling converts to float64
}

func TestEncryptedCheckpointer_AlgorithmValidation(t *testing.T) {
	ctx := context.Background()
	base := checkpoint.NewInMemoryCheckpointer()
	key := []byte("12345678901234567890123456789012")

	t.Run("algorithm reported correctly", func(t *testing.T) {
		encryptor, err := checkpoint.NewAES256GCMEncryptor(key)
		require.NoError(t, err)

		assert.Equal(t, "aes-256-gcm", encryptor.Algorithm())

		encrypted, err := checkpoint.NewEncryptedCheckpointer(base, encryptor)
		require.NoError(t, err)

		cp := &checkpoint.Checkpoint{
			RunID:     "test-algo",
			Superstep: 1,
			State:     map[string]any{"data": "test"},
		}

		err = encrypted.Save(ctx, cp)
		require.NoError(t, err)

		// Verify algorithm is set
		rawCP, err := base.Load(ctx, "test-algo")
		require.NoError(t, err)
		assert.Equal(t, "aes-256-gcm", rawCP.Metadata["algorithm"])

		// Verify can load
		loadedCP, err := encrypted.Load(ctx, "test-algo")
		require.NoError(t, err)
		assert.Equal(t, "test", loadedCP.State["data"])
	})

	t.Run("algorithm mismatch detection", func(t *testing.T) {
		encryptor, err := checkpoint.NewAES256GCMEncryptor(key)
		require.NoError(t, err)

		enc1, err := checkpoint.NewEncryptedCheckpointer(base, encryptor)
		require.NoError(t, err)

		cp := &checkpoint.Checkpoint{
			RunID:     "test-mismatch",
			Superstep: 1,
			State:     map[string]any{"data": "test"},
		}

		err = enc1.Save(ctx, cp)
		require.NoError(t, err)

		// Manually modify the algorithm in metadata to simulate mismatch
		rawCP, err := base.Load(ctx, "test-mismatch")
		require.NoError(t, err)
		rawCP.Metadata["algorithm"] = "different-algorithm"
		err = base.Save(ctx, rawCP)
		require.NoError(t, err)

		// Try to load - should fail with algorithm mismatch
		_, err = enc1.Load(ctx, "test-mismatch")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "encrypted with different-algorithm")
		assert.Contains(t, err.Error(), "configured for aes-256-gcm")
	})
}
