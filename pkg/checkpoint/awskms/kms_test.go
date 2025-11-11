package awskms

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockKMSClient is a mock implementation of the Client interface
type MockKMSClient struct {
	mock.Mock
}

func (m *MockKMSClient) Encrypt(ctx context.Context, params *kms.EncryptInput, optFns ...func(*kms.Options)) (*kms.EncryptOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*kms.EncryptOutput), args.Error(1)
}

func (m *MockKMSClient) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*kms.DecryptOutput), args.Error(1)
}

// MockCheckpointer is a mock implementation of checkpoint.Checkpointer
type MockCheckpointer struct {
	mock.Mock
}

func (m *MockCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	args := m.Called(ctx, cp)
	return args.Error(0)
}

func (m *MockCheckpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	args := m.Called(ctx, runID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*checkpoint.Checkpoint), args.Error(1)
}

func (m *MockCheckpointer) List(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error) {
	args := m.Called(ctx, runID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*checkpoint.Checkpoint), args.Error(1)
}

func (m *MockCheckpointer) Delete(ctx context.Context, runID string) error {
	args := m.Called(ctx, runID)
	return args.Error(0)
}

func (m *MockCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	args := m.Called(ctx, runID, superstep)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*checkpoint.Checkpoint), args.Error(1)
}

func TestNewKMSCheckpointer(t *testing.T) {
	mockBase := &MockCheckpointer{}
	mockKMS := &MockKMSClient{}

	tests := []struct {
		name      string
		base      checkpoint.Checkpointer
		kmsClient Client
		keyID     string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid configuration",
			base:      mockBase,
			kmsClient: mockKMS,
			keyID:     "test-key-id",
			wantErr:   false,
		},
		{
			name:      "missing key ID",
			base:      mockBase,
			kmsClient: mockKMS,
			keyID:     "",
			wantErr:   true,
			errMsg:    "KMS key ID is required",
		},
		{
			name:      "nil KMS client",
			base:      mockBase,
			kmsClient: nil,
			keyID:     "test-key-id",
			wantErr:   true,
			errMsg:    "KMS client is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kmsCP, err := NewKMSCheckpointer(tt.base, tt.kmsClient, tt.keyID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, kmsCP)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, kmsCP)
				assert.Equal(t, tt.base, kmsCP.base)
				assert.Equal(t, tt.kmsClient, kmsCP.kmsClient)
				assert.Equal(t, tt.keyID, kmsCP.keyID)
			}
		})
	}
}

func TestKMSCheckpointer_Save(t *testing.T) {
	ctx := context.Background()
	keyID := "test-key-id"

	originalCP := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Metadata: map[string]any{
			"created_at": "2025-11-11T00:00:00Z",
		},
	}

	// Serialize the original checkpoint
	originalData, err := json.Marshal(originalCP)
	require.NoError(t, err)

	t.Run("successful encryption and save", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		// Mock KMS Encrypt
		encryptedBlob := []byte("encrypted-data")
		mockKMS.On("Encrypt", ctx, mock.MatchedBy(func(input *kms.EncryptInput) bool {
			return *input.KeyId == keyID && string(input.Plaintext) == string(originalData)
		})).Return(&kms.EncryptOutput{
			CiphertextBlob: encryptedBlob,
		}, nil)

		// Mock base Save
		mockBase.On("Save", ctx, mock.MatchedBy(func(cp *checkpoint.Checkpoint) bool {
			encryptedKMS, ok := cp.Metadata["encrypted_kms"].(bool)
			if !ok || !encryptedKMS {
				return false
			}
			keyIDMeta, ok := cp.Metadata["key_id"].(string)
			if !ok || keyIDMeta != keyID {
				return false
			}
			payload, ok := cp.Metadata["payload"].(string)
			if !ok {
				return false
			}
			decoded, _ := base64.StdEncoding.DecodeString(payload)
			return string(decoded) == string(encryptedBlob)
		})).Return(nil)

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		err = kmsCP.Save(ctx, originalCP)
		assert.NoError(t, err)

		mockKMS.AssertExpectations(t)
		mockBase.AssertExpectations(t)
	})

	t.Run("KMS encryption fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		mockKMS.On("Encrypt", ctx, mock.Anything).Return(nil, errors.New("KMS error"))

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		err = kmsCP.Save(ctx, originalCP)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "KMS encryption failed")

		mockKMS.AssertExpectations(t)
	})

	t.Run("base save fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		mockKMS.On("Encrypt", ctx, mock.Anything).Return(&kms.EncryptOutput{
			CiphertextBlob: []byte("encrypted-data"),
		}, nil)

		mockBase.On("Save", ctx, mock.Anything).Return(errors.New("save error"))

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		err = kmsCP.Save(ctx, originalCP)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "save error")

		mockKMS.AssertExpectations(t)
		mockBase.AssertExpectations(t)
	})
}

func TestKMSCheckpointer_Load(t *testing.T) {
	ctx := context.Background()
	keyID := "test-key-id"
	runID := "test-run"

	originalCP := &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: 1,
		Metadata: map[string]any{
			"created_at": "2025-11-11T00:00:00Z",
		},
	}

	originalData, err := json.Marshal(originalCP)
	require.NoError(t, err)

	encryptedBlob := []byte("encrypted-data")
	encodedPayload := base64.StdEncoding.EncodeToString(encryptedBlob)

	t.Run("successful load and decrypt", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"encrypted_kms": true,
				"key_id":        keyID,
				"payload":       encodedPayload,
			},
		}

		mockBase.On("Load", ctx, runID).Return(encryptedCP, nil)

		mockKMS.On("Decrypt", ctx, mock.MatchedBy(func(input *kms.DecryptInput) bool {
			return *input.KeyId == keyID && string(input.CiphertextBlob) == string(encryptedBlob)
		})).Return(&kms.DecryptOutput{
			Plaintext: originalData,
		}, nil)

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		cp, err := kmsCP.Load(ctx, runID)
		assert.NoError(t, err)
		assert.NotNil(t, cp)
		assert.Equal(t, runID, cp.RunID)
		assert.Equal(t, int64(1), cp.Superstep)

		mockBase.AssertExpectations(t)
		mockKMS.AssertExpectations(t)
	})

	t.Run("load unencrypted checkpoint", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		unencryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"created_at": "2025-11-11T00:00:00Z",
			},
		}

		mockBase.On("Load", ctx, runID).Return(unencryptedCP, nil)

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		cp, err := kmsCP.Load(ctx, runID)
		assert.NoError(t, err)
		assert.Equal(t, unencryptedCP, cp)

		mockBase.AssertExpectations(t)
	})

	t.Run("base load fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		mockBase.On("Load", ctx, runID).Return(nil, errors.New("load error"))

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		cp, err := kmsCP.Load(ctx, runID)
		assert.Error(t, err)
		assert.Nil(t, cp)
		assert.Contains(t, err.Error(), "load error")

		mockBase.AssertExpectations(t)
	})

	t.Run("KMS decryption fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"encrypted_kms": true,
				"key_id":        keyID,
				"payload":       encodedPayload,
			},
		}

		mockBase.On("Load", ctx, runID).Return(encryptedCP, nil)
		mockKMS.On("Decrypt", ctx, mock.Anything).Return(nil, errors.New("KMS decrypt error"))

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		cp, err := kmsCP.Load(ctx, runID)
		assert.Error(t, err)
		assert.Nil(t, cp)
		assert.Contains(t, err.Error(), "KMS decryption failed")

		mockBase.AssertExpectations(t)
		mockKMS.AssertExpectations(t)
	})

	t.Run("missing payload in encrypted checkpoint", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"encrypted_kms": true,
				"key_id":        keyID,
				// missing payload
			},
		}

		mockBase.On("Load", ctx, runID).Return(encryptedCP, nil)

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		cp, err := kmsCP.Load(ctx, runID)
		assert.Error(t, err)
		assert.Nil(t, cp)
		assert.Contains(t, err.Error(), "missing payload")

		mockBase.AssertExpectations(t)
	})

	t.Run("invalid base64 payload", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"encrypted_kms": true,
				"key_id":        keyID,
				"payload":       "invalid-base64!!!",
			},
		}

		mockBase.On("Load", ctx, runID).Return(encryptedCP, nil)

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		cp, err := kmsCP.Load(ctx, runID)
		assert.Error(t, err)
		assert.Nil(t, cp)
		assert.Contains(t, err.Error(), "failed to decode encrypted payload")

		mockBase.AssertExpectations(t)
	})
}

func TestKMSCheckpointer_List(t *testing.T) {
	ctx := context.Background()
	keyID := "test-key-id"
	runID := "test-run"

	expectedCheckpoints := []*checkpoint.Checkpoint{
		{RunID: runID, Superstep: 1},
		{RunID: runID, Superstep: 2},
	}

	mockBase := &MockCheckpointer{}
	mockKMS := &MockKMSClient{}

	mockBase.On("List", ctx, runID).Return(expectedCheckpoints, nil)

	kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
	require.NoError(t, err)

	checkpoints, err := kmsCP.List(ctx, runID)
	assert.NoError(t, err)
	assert.Equal(t, expectedCheckpoints, checkpoints)

	mockBase.AssertExpectations(t)
}

func TestKMSCheckpointer_Delete(t *testing.T) {
	ctx := context.Background()
	keyID := "test-key-id"
	runID := "test-run"

	mockBase := &MockCheckpointer{}
	mockKMS := &MockKMSClient{}

	mockBase.On("Delete", ctx, runID).Return(nil)

	kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
	require.NoError(t, err)

	err = kmsCP.Delete(ctx, runID)
	assert.NoError(t, err)

	mockBase.AssertExpectations(t)
}

func TestKMSCheckpointer_LoadAtSuperstep(t *testing.T) {
	ctx := context.Background()
	keyID := "test-key-id"
	runID := "test-run"
	superstep := int64(5)

	originalCP := &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: superstep,
		Metadata: map[string]any{
			"created_at": "2025-11-11T00:00:00Z",
		},
	}

	originalData, err := json.Marshal(originalCP)
	require.NoError(t, err)

	encryptedBlob := []byte("encrypted-data")
	encodedPayload := base64.StdEncoding.EncodeToString(encryptedBlob)

	t.Run("successful load at superstep with decryption", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: superstep,
			Metadata: map[string]any{
				"encrypted_kms": true,
				"key_id":        keyID,
				"payload":       encodedPayload,
			},
		}

		mockBase.On("LoadAtSuperstep", ctx, runID, superstep).Return(encryptedCP, nil)
		mockKMS.On("Decrypt", ctx, mock.Anything).Return(&kms.DecryptOutput{
			Plaintext: originalData,
		}, nil)

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		cp, err := kmsCP.LoadAtSuperstep(ctx, runID, superstep)
		assert.NoError(t, err)
		assert.NotNil(t, cp)
		assert.Equal(t, runID, cp.RunID)
		assert.Equal(t, superstep, cp.Superstep)

		mockBase.AssertExpectations(t)
		mockKMS.AssertExpectations(t)
	})

	t.Run("load unencrypted checkpoint at superstep", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		unencryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: superstep,
			Metadata: map[string]any{
				"created_at": "2025-11-11T00:00:00Z",
			},
		}

		mockBase.On("LoadAtSuperstep", ctx, runID, superstep).Return(unencryptedCP, nil)

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		cp, err := kmsCP.LoadAtSuperstep(ctx, runID, superstep)
		assert.NoError(t, err)
		assert.Equal(t, unencryptedCP, cp)

		mockBase.AssertExpectations(t)
	})

	t.Run("base LoadAtSuperstep fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockKMS := &MockKMSClient{}

		mockBase.On("LoadAtSuperstep", ctx, runID, superstep).Return(nil, errors.New("load error"))

		kmsCP, err := NewKMSCheckpointer(mockBase, mockKMS, keyID)
		require.NoError(t, err)

		cp, err := kmsCP.LoadAtSuperstep(ctx, runID, superstep)
		assert.Error(t, err)
		assert.Nil(t, cp)

		mockBase.AssertExpectations(t)
	})
}
