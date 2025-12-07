package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	vault "github.com/hashicorp/vault/api"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Client is an interface for Vault logical operations, allowing for testing and mocking.
type Client interface {
	Logical() LogicalClient
}

// LogicalClient is an interface for Vault logical backend operations.
type LogicalClient interface {
	WriteWithContext(ctx context.Context, path string, data map[string]interface{}) (*vault.Secret, error)
}

// VaultCheckpointer wraps a base checkpointer with HashiCorp Vault transit encryption.
// Note: The name VaultCheckpointer is intentionally used (despite stuttering) for consistency
// with KMSCheckpointer and to clearly distinguish from the base checkpoint.Checkpointer.
//
//nolint:revive // Intentional naming for API consistency
type VaultCheckpointer struct {
	base      checkpoint.Checkpointer
	client    Client
	mountPath string
	keyName   string
}

// Options configures optional parameters for VaultCheckpointer
type Options struct {
	// MountPath is the transit secrets engine mount path (default: "transit")
	MountPath string
}

// Option is a functional option for configuring VaultCheckpointer
type Option func(*Options)

// WithMountPath sets the transit secrets engine mount path
func WithMountPath(mountPath string) Option {
	return func(o *Options) {
		o.MountPath = mountPath
	}
}

// NewVaultCheckpointer creates a new Vault-based checkpointer.
// It wraps an existing checkpointer with Vault transit encryption.
//
// Parameters:
//   - base: The underlying checkpointer to wrap
//   - client: Vault client interface for transit operations
//   - keyName: Name of the transit encryption key in Vault
//   - opts: Optional configuration (e.g., WithMountPath)
//
// Example:
//
//	client, _ := vault.NewClient(vault.DefaultConfig())
//	vc, err := NewVaultCheckpointer(
//	    memoryCheckpointer,
//	    client,
//	    "my-encryption-key",
//	    WithMountPath("transit"),
//	)
func NewVaultCheckpointer(base checkpoint.Checkpointer, client Client, keyName string, opts ...Option) (*VaultCheckpointer, error) {
	if client == nil {
		return nil, ErrClientRequired
	}

	if keyName == "" {
		return nil, ErrKeyNameRequired
	}

	// Apply default options
	options := &Options{
		MountPath: "transit",
	}

	// Apply user options
	for _, opt := range opts {
		opt(options)
	}

	return &VaultCheckpointer{
		base:      base,
		client:    client,
		mountPath: options.MountPath,
		keyName:   keyName,
	}, nil
}

// Save encrypts and saves a checkpoint using Vault transit encryption.
// The checkpoint data is first serialized, then encrypted via Vault's transit engine,
// and finally stored using the base checkpointer.
func (vc *VaultCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	// Serialize checkpoint
	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Encrypt with Vault transit
	path := fmt.Sprintf("%s/encrypt/%s", vc.mountPath, vc.keyName)
	secret, err := vc.client.Logical().WriteWithContext(ctx, path, map[string]interface{}{
		"plaintext": base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return fmt.Errorf("vault encryption failed: %w", err)
	}

	ciphertext, ok := secret.Data["ciphertext"].(string)
	if !ok {
		return fmt.Errorf("vault: %w", ErrMissingCiphertext)
	}

	// Create wrapper checkpoint with encrypted payload
	encryptedCP := &checkpoint.Checkpoint{
		RunID:     cp.RunID,
		Superstep: cp.Superstep,
		Metadata: map[string]any{
			"encrypted_vault": true,
			"mount_path":      vc.mountPath,
			"key_name":        vc.keyName,
			"ciphertext":      ciphertext,
			"created_at":      cp.Metadata["created_at"],
		},
	}

	return vc.base.Save(ctx, encryptedCP)
}

// Load retrieves and decrypts a checkpoint using Vault transit decryption.
// It loads the encrypted checkpoint from the base checkpointer, decrypts it via Vault,
// and returns the original checkpoint data.
func (vc *VaultCheckpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	// Load potentially encrypted checkpoint
	encryptedCP, err := vc.base.Load(ctx, runID)
	if err != nil {
		return nil, err
	}

	// Check if encrypted with Vault
	encryptedVault, ok := encryptedCP.Metadata["encrypted_vault"].(bool)
	if !ok || !encryptedVault {
		// Not Vault encrypted, return as-is
		return encryptedCP, nil
	}

	// Get ciphertext
	ciphertext, ok := encryptedCP.Metadata["ciphertext"].(string)
	if !ok {
		return nil, ErrMissingCiphertext
	}

	// Decrypt with Vault transit
	path := fmt.Sprintf("%s/decrypt/%s", vc.mountPath, vc.keyName)
	secret, err := vc.client.Logical().WriteWithContext(ctx, path, map[string]interface{}{
		"ciphertext": ciphertext,
	})
	if err != nil {
		return nil, fmt.Errorf("vault decryption failed: %w", err)
	}

	plaintext, ok := secret.Data["plaintext"].(string)
	if !ok {
		return nil, ErrMissingPlaintext
	}

	// Decode base64
	data, err := base64.StdEncoding.DecodeString(plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode plaintext: %w", err)
	}

	// Unmarshal original checkpoint
	cp := &checkpoint.Checkpoint{}
	if err := json.Unmarshal(data, cp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return cp, nil
}

// List returns all checkpoints for the given runID by delegating to the base checkpointer.
// Note: The returned list contains metadata only; actual checkpoint data requires Load().
func (vc *VaultCheckpointer) List(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error) {
	return vc.base.List(ctx, runID)
}

// Delete removes all checkpoints for the given runID by delegating to the base checkpointer.
func (vc *VaultCheckpointer) Delete(ctx context.Context, runID string) error {
	return vc.base.Delete(ctx, runID)
}

// LoadAtSuperstep retrieves and decrypts a checkpoint at a specific superstep using Vault transit decryption.
// It loads the encrypted checkpoint from the base checkpointer, decrypts it via Vault,
// and returns the original checkpoint data.
func (vc *VaultCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	cp, err := vc.base.LoadAtSuperstep(ctx, runID, superstep)
	if err != nil {
		return nil, err
	}

	// Check if encrypted with Vault
	encryptedVault, ok := cp.Metadata["encrypted_vault"].(bool)
	if !ok || !encryptedVault {
		return cp, nil
	}

	// Decrypt (reuse Load logic)
	ciphertext, ok := cp.Metadata["ciphertext"].(string)
	if !ok {
		return nil, ErrMissingCiphertext
	}

	path := fmt.Sprintf("%s/decrypt/%s", vc.mountPath, vc.keyName)
	secret, err := vc.client.Logical().WriteWithContext(ctx, path, map[string]interface{}{
		"ciphertext": ciphertext,
	})
	if err != nil {
		return nil, fmt.Errorf("vault decryption failed: %w", err)
	}

	plaintext, ok := secret.Data["plaintext"].(string)
	if !ok {
		return nil, ErrMissingPlaintext
	}

	data, err := base64.StdEncoding.DecodeString(plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode plaintext: %w", err)
	}

	checkpoint := &checkpoint.Checkpoint{}
	if err := json.Unmarshal(data, checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return checkpoint, nil
}
