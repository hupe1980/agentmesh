package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	vault "github.com/hashicorp/vault/api"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockVaultClient is a mock implementation of the Client interface
type MockVaultClient struct {
	mock.Mock
	logicalClient *MockLogicalClient
}

func (m *MockVaultClient) Logical() LogicalClient {
	return m.logicalClient
}

// MockLogicalClient is a mock implementation of LogicalClient
type MockLogicalClient struct {
	mock.Mock
}

func (m *MockLogicalClient) WriteWithContext(ctx context.Context, path string, data map[string]interface{}) (*vault.Secret, error) {
	args := m.Called(ctx, path, data)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*vault.Secret), args.Error(1)
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

func (m *MockCheckpointer) ListPendingApprovals(ctx context.Context) ([]*checkpoint.Checkpoint, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*checkpoint.Checkpoint), args.Error(1)
}

func (m *MockCheckpointer) GetApprovalHistory(ctx context.Context, runID string) ([]checkpoint.ApprovalRecord, error) {
	args := m.Called(ctx, runID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]checkpoint.ApprovalRecord), args.Error(1)
}

func TestNewVaultCheckpointer(t *testing.T) {
	mockBase := &MockCheckpointer{}
	mockLogical := &MockLogicalClient{}
	mockVault := &MockVaultClient{logicalClient: mockLogical}

	tests := []struct {
		name      string
		base      checkpoint.Checkpointer
		client    Client
		keyName   string
		opts      []Option
		wantErr   bool
		errMsg    string
		checkFunc func(t *testing.T, vc *VaultCheckpointer)
	}{
		{
			name:    "valid configuration with defaults",
			base:    mockBase,
			client:  mockVault,
			keyName: "test-key",
			wantErr: false,
			checkFunc: func(t *testing.T, vc *VaultCheckpointer) {
				assert.Equal(t, "transit", vc.mountPath)
				assert.Equal(t, "test-key", vc.keyName)
			},
		},
		{
			name:    "valid configuration with custom mount path",
			base:    mockBase,
			client:  mockVault,
			keyName: "test-key",
			opts:    []Option{WithMountPath("custom-transit")},
			wantErr: false,
			checkFunc: func(t *testing.T, vc *VaultCheckpointer) {
				assert.Equal(t, "custom-transit", vc.mountPath)
				assert.Equal(t, "test-key", vc.keyName)
			},
		},
		{
			name:    "missing key name",
			base:    mockBase,
			client:  mockVault,
			keyName: "",
			wantErr: true,
			errMsg:  "vault key name is required",
		},
		{
			name:    "nil vault client",
			base:    mockBase,
			client:  nil,
			keyName: "test-key",
			wantErr: true,
			errMsg:  "vault client is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc, err := NewVaultCheckpointer(tt.base, tt.client, tt.keyName, tt.opts...)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, vc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, vc)
				assert.Equal(t, tt.base, vc.base)
				assert.Equal(t, tt.client, vc.client)
				if tt.checkFunc != nil {
					tt.checkFunc(t, vc)
				}
			}
		})
	}
}

func TestVaultCheckpointer_Save(t *testing.T) {
	ctx := context.Background()
	keyName := "test-key"

	originalCP := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Metadata: map[string]any{
			"created_at": "2025-11-11T00:00:00Z",
		},
	}

	originalData, err := json.Marshal(originalCP)
	require.NoError(t, err)
	encodedData := base64.StdEncoding.EncodeToString(originalData)

	t.Run("successful encryption and save", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		ciphertext := "vault:v1:encrypted-data"

		// Mock Vault encrypt
		mockLogical.On("WriteWithContext", ctx, "transit/encrypt/test-key", map[string]interface{}{
			"plaintext": encodedData,
		}).Return(&vault.Secret{
			Data: map[string]interface{}{
				"ciphertext": ciphertext,
			},
		}, nil)

		// Mock base Save
		mockBase.On("Save", ctx, mock.MatchedBy(func(cp *checkpoint.Checkpoint) bool {
			encryptedVault, ok := cp.Metadata["encrypted_vault"].(bool)
			if !ok || !encryptedVault {
				return false
			}
			ct, ok := cp.Metadata["ciphertext"].(string)
			return ok && ct == ciphertext
		})).Return(nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		err = vc.Save(ctx, originalCP)
		assert.NoError(t, err)

		mockLogical.AssertExpectations(t)
		mockBase.AssertExpectations(t)
	})

	t.Run("vault encryption fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		mockLogical.On("WriteWithContext", ctx, mock.Anything, mock.Anything).
			Return(nil, errors.New("vault error"))

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		err = vc.Save(ctx, originalCP)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "vault encryption failed")

		mockLogical.AssertExpectations(t)
	})

	t.Run("vault response missing ciphertext", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		mockLogical.On("WriteWithContext", ctx, mock.Anything, mock.Anything).
			Return(&vault.Secret{
				Data: map[string]interface{}{},
			}, nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		err = vc.Save(ctx, originalCP)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing ciphertext")

		mockLogical.AssertExpectations(t)
	})

	t.Run("base save fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		mockLogical.On("WriteWithContext", ctx, mock.Anything, mock.Anything).
			Return(&vault.Secret{
				Data: map[string]interface{}{
					"ciphertext": "vault:v1:encrypted",
				},
			}, nil)

		mockBase.On("Save", ctx, mock.Anything).Return(errors.New("save error"))

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		err = vc.Save(ctx, originalCP)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "save error")

		mockLogical.AssertExpectations(t)
		mockBase.AssertExpectations(t)
	})

	t.Run("with custom mount path", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		customMount := "custom-transit"
		ciphertext := "vault:v1:encrypted-data"

		mockLogical.On("WriteWithContext", ctx, customMount+"/encrypt/"+keyName, mock.Anything).
			Return(&vault.Secret{
				Data: map[string]interface{}{
					"ciphertext": ciphertext,
				},
			}, nil)

		mockBase.On("Save", ctx, mock.Anything).Return(nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName, WithMountPath(customMount))
		require.NoError(t, err)

		err = vc.Save(ctx, originalCP)
		assert.NoError(t, err)

		mockLogical.AssertExpectations(t)
		mockBase.AssertExpectations(t)
	})
}

func TestVaultCheckpointer_Load(t *testing.T) {
	ctx := context.Background()
	keyName := "test-key"
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
	encodedData := base64.StdEncoding.EncodeToString(originalData)
	ciphertext := "vault:v1:encrypted-data"

	t.Run("successful load and decrypt", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"encrypted_vault": true,
				"mount_path":      "transit",
				"key_name":        keyName,
				"ciphertext":      ciphertext,
			},
		}

		mockBase.On("Load", ctx, runID).Return(encryptedCP, nil)

		mockLogical.On("WriteWithContext", ctx, "transit/decrypt/test-key", map[string]interface{}{
			"ciphertext": ciphertext,
		}).Return(&vault.Secret{
			Data: map[string]interface{}{
				"plaintext": encodedData,
			},
		}, nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.Load(ctx, runID)
		assert.NoError(t, err)
		assert.NotNil(t, cp)
		assert.Equal(t, runID, cp.RunID)
		assert.Equal(t, int64(1), cp.Superstep)

		mockBase.AssertExpectations(t)
		mockLogical.AssertExpectations(t)
	})

	t.Run("load unencrypted checkpoint", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		unencryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"created_at": "2025-11-11T00:00:00Z",
			},
		}

		mockBase.On("Load", ctx, runID).Return(unencryptedCP, nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.Load(ctx, runID)
		assert.NoError(t, err)
		assert.Equal(t, unencryptedCP, cp)

		mockBase.AssertExpectations(t)
	})

	t.Run("base load fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		mockBase.On("Load", ctx, runID).Return(nil, errors.New("load error"))

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.Load(ctx, runID)
		assert.Error(t, err)
		assert.Nil(t, cp)
		assert.Contains(t, err.Error(), "load error")

		mockBase.AssertExpectations(t)
	})

	t.Run("vault decryption fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"encrypted_vault": true,
				"ciphertext":      ciphertext,
			},
		}

		mockBase.On("Load", ctx, runID).Return(encryptedCP, nil)
		mockLogical.On("WriteWithContext", ctx, mock.Anything, mock.Anything).
			Return(nil, errors.New("vault decrypt error"))

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.Load(ctx, runID)
		assert.Error(t, err)
		assert.Nil(t, cp)
		assert.Contains(t, err.Error(), "vault decryption failed")

		mockBase.AssertExpectations(t)
		mockLogical.AssertExpectations(t)
	})

	t.Run("missing ciphertext in encrypted checkpoint", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"encrypted_vault": true,
			},
		}

		mockBase.On("Load", ctx, runID).Return(encryptedCP, nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.Load(ctx, runID)
		assert.Error(t, err)
		assert.Nil(t, cp)
		assert.Contains(t, err.Error(), "missing ciphertext")

		mockBase.AssertExpectations(t)
	})

	t.Run("vault response missing plaintext", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"encrypted_vault": true,
				"ciphertext":      ciphertext,
			},
		}

		mockBase.On("Load", ctx, runID).Return(encryptedCP, nil)
		mockLogical.On("WriteWithContext", ctx, mock.Anything, mock.Anything).
			Return(&vault.Secret{
				Data: map[string]interface{}{},
			}, nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.Load(ctx, runID)
		assert.Error(t, err)
		assert.Nil(t, cp)
		assert.Contains(t, err.Error(), "missing plaintext")

		mockBase.AssertExpectations(t)
		mockLogical.AssertExpectations(t)
	})

	t.Run("invalid base64 plaintext", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: 1,
			Metadata: map[string]any{
				"encrypted_vault": true,
				"ciphertext":      ciphertext,
			},
		}

		mockBase.On("Load", ctx, runID).Return(encryptedCP, nil)
		mockLogical.On("WriteWithContext", ctx, mock.Anything, mock.Anything).
			Return(&vault.Secret{
				Data: map[string]interface{}{
					"plaintext": "invalid-base64!!!",
				},
			}, nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.Load(ctx, runID)
		assert.Error(t, err)
		assert.Nil(t, cp)
		assert.Contains(t, err.Error(), "failed to decode plaintext")

		mockBase.AssertExpectations(t)
		mockLogical.AssertExpectations(t)
	})
}

func TestVaultCheckpointer_List(t *testing.T) {
	ctx := context.Background()
	keyName := "test-key"
	runID := "test-run"

	expectedCheckpoints := []*checkpoint.Checkpoint{
		{RunID: runID, Superstep: 1},
		{RunID: runID, Superstep: 2},
	}

	mockBase := &MockCheckpointer{}
	mockLogical := &MockLogicalClient{}
	mockVault := &MockVaultClient{logicalClient: mockLogical}

	mockBase.On("List", ctx, runID).Return(expectedCheckpoints, nil)

	vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
	require.NoError(t, err)

	checkpoints, err := vc.List(ctx, runID)
	assert.NoError(t, err)
	assert.Equal(t, expectedCheckpoints, checkpoints)

	mockBase.AssertExpectations(t)
}

func TestVaultCheckpointer_Delete(t *testing.T) {
	ctx := context.Background()
	keyName := "test-key"
	runID := "test-run"

	mockBase := &MockCheckpointer{}
	mockLogical := &MockLogicalClient{}
	mockVault := &MockVaultClient{logicalClient: mockLogical}

	mockBase.On("Delete", ctx, runID).Return(nil)

	vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
	require.NoError(t, err)

	err = vc.Delete(ctx, runID)
	assert.NoError(t, err)

	mockBase.AssertExpectations(t)
}

func TestVaultCheckpointer_LoadAtSuperstep(t *testing.T) {
	ctx := context.Background()
	keyName := "test-key"
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
	encodedData := base64.StdEncoding.EncodeToString(originalData)
	ciphertext := "vault:v1:encrypted-data"

	t.Run("successful load at superstep with decryption", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		encryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: superstep,
			Metadata: map[string]any{
				"encrypted_vault": true,
				"ciphertext":      ciphertext,
			},
		}

		mockBase.On("LoadAtSuperstep", ctx, runID, superstep).Return(encryptedCP, nil)
		mockLogical.On("WriteWithContext", ctx, mock.Anything, mock.Anything).
			Return(&vault.Secret{
				Data: map[string]interface{}{
					"plaintext": encodedData,
				},
			}, nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.LoadAtSuperstep(ctx, runID, superstep)
		assert.NoError(t, err)
		assert.NotNil(t, cp)
		assert.Equal(t, runID, cp.RunID)
		assert.Equal(t, superstep, cp.Superstep)

		mockBase.AssertExpectations(t)
		mockLogical.AssertExpectations(t)
	})

	t.Run("load unencrypted checkpoint at superstep", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		unencryptedCP := &checkpoint.Checkpoint{
			RunID:     runID,
			Superstep: superstep,
			Metadata: map[string]any{
				"created_at": "2025-11-11T00:00:00Z",
			},
		}

		mockBase.On("LoadAtSuperstep", ctx, runID, superstep).Return(unencryptedCP, nil)

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.LoadAtSuperstep(ctx, runID, superstep)
		assert.NoError(t, err)
		assert.Equal(t, unencryptedCP, cp)

		mockBase.AssertExpectations(t)
	})

	t.Run("base LoadAtSuperstep fails", func(t *testing.T) {
		mockBase := &MockCheckpointer{}
		mockLogical := &MockLogicalClient{}
		mockVault := &MockVaultClient{logicalClient: mockLogical}

		mockBase.On("LoadAtSuperstep", ctx, runID, superstep).Return(nil, errors.New("load error"))

		vc, err := NewVaultCheckpointer(mockBase, mockVault, keyName)
		require.NoError(t, err)

		cp, err := vc.LoadAtSuperstep(ctx, runID, superstep)
		assert.Error(t, err)
		assert.Nil(t, cp)

		mockBase.AssertExpectations(t)
	})
}

func TestWithMountPath(t *testing.T) {
	mockBase := &MockCheckpointer{}
	mockLogical := &MockLogicalClient{}
	mockVault := &MockVaultClient{logicalClient: mockLogical}

	customMount := "custom-mount"
	vc, err := NewVaultCheckpointer(mockBase, mockVault, "test-key", WithMountPath(customMount))

	require.NoError(t, err)
	assert.Equal(t, customMount, vc.mountPath)
}
