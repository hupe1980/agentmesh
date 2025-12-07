package awskms

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Client is an interface for KMS operations, allowing for testing and mocking.
type Client interface {
	Encrypt(ctx context.Context, params *kms.EncryptInput, optFns ...func(*kms.Options)) (*kms.EncryptOutput, error)
	Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// KMSCheckpointer wraps a base checkpointer with AWS KMS encryption
type KMSCheckpointer struct {
	base      checkpoint.Checkpointer
	kmsClient Client
	keyID     string
}

// NewKMSCheckpointer creates a checkpointer that uses AWS KMS for encryption.
// The kmsClient should be created from your AWS configuration, and keyID can be:
//   - Key ID: "12345678-1234-1234-1234-123456789012"
//   - Key ARN: "arn:aws:kms:us-east-1:123456789012:key/..."
//   - Alias: "alias/my-key"
//   - Alias ARN: "arn:aws:kms:us-east-1:123456789012:alias/my-key"
func NewKMSCheckpointer(base checkpoint.Checkpointer, kmsClient Client, keyID string) (*KMSCheckpointer, error) {
	if keyID == "" {
		return nil, ErrKeyIDRequired
	}

	if kmsClient == nil {
		return nil, ErrClientRequired
	}

	return &KMSCheckpointer{
		base:      base,
		kmsClient: kmsClient,
		keyID:     keyID,
	}, nil
}

// Save encrypts and saves a checkpoint using AWS KMS encryption.
// The checkpoint data is first serialized, then encrypted with the KMS key,
// and finally stored using the base checkpointer.
func (kc *KMSCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	// Serialize checkpoint
	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Encrypt with KMS
	result, err := kc.kmsClient.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(kc.keyID),
		Plaintext: data,
	})
	if err != nil {
		return fmt.Errorf("kms encryption failed: %w", err)
	}

	// Create wrapper checkpoint with encrypted payload
	encryptedCP := &checkpoint.Checkpoint{
		RunID:     cp.RunID,
		Superstep: cp.Superstep,
		Metadata: map[string]any{
			"encrypted_kms": true,
			"key_id":        kc.keyID,
			"payload":       base64.StdEncoding.EncodeToString(result.CiphertextBlob),
			"created_at":    cp.Metadata["created_at"],
		},
	}

	return kc.base.Save(ctx, encryptedCP)
}

// Load retrieves and decrypts a checkpoint using AWS KMS.
// It loads the encrypted checkpoint from the base checkpointer, decrypts it with KMS,
// and returns the original checkpoint data.
func (kc *KMSCheckpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	// Load potentially encrypted checkpoint
	encryptedCP, err := kc.base.Load(ctx, runID)
	if err != nil {
		return nil, err
	}

	// Check if encrypted with KMS
	encryptedKMS, ok := encryptedCP.Metadata["encrypted_kms"].(bool)
	if !ok || !encryptedKMS {
		// Not KMS encrypted, return as-is
		return encryptedCP, nil
	}

	// Decode base64 payload
	payloadStr, ok := encryptedCP.Metadata["payload"].(string)
	if !ok {
		return nil, ErrMissingPayload
	}

	encryptedData, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted payload: %w", err)
	}

	// Decrypt with KMS
	result, err := kc.kmsClient.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: encryptedData,
		KeyId:          aws.String(kc.keyID),
	})
	if err != nil {
		return nil, fmt.Errorf("kms decryption failed: %w", err)
	}

	// Unmarshal original checkpoint
	cp := &checkpoint.Checkpoint{}
	if err := json.Unmarshal(result.Plaintext, cp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return cp, nil
}

// List returns all checkpoints for the given runID by delegating to the base checkpointer.
// Note: The returned list contains metadata only; actual checkpoint data requires Load().
func (kc *KMSCheckpointer) List(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error) {
	return kc.base.List(ctx, runID)
}

// Delete removes all checkpoints for the given runID by delegating to the base checkpointer.
func (kc *KMSCheckpointer) Delete(ctx context.Context, runID string) error {
	return kc.base.Delete(ctx, runID)
}

// LoadAtSuperstep retrieves and decrypts a checkpoint at a specific superstep using AWS KMS.
// It loads the encrypted checkpoint from the base checkpointer, decrypts it with KMS,
// and returns the original checkpoint data.
func (kc *KMSCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	cp, err := kc.base.LoadAtSuperstep(ctx, runID, superstep)
	if err != nil {
		return nil, err
	}

	// Check if encrypted with KMS
	encryptedKMS, ok := cp.Metadata["encrypted_kms"].(bool)
	if !ok || !encryptedKMS {
		return cp, nil
	}

	// Decrypt (reuse Load logic)
	payloadStr, ok := cp.Metadata["payload"].(string)
	if !ok {
		return nil, ErrMissingPayload
	}

	encryptedData, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted payload: %w", err)
	}

	result, err := kc.kmsClient.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: encryptedData,
		KeyId:          aws.String(kc.keyID),
	})
	if err != nil {
		return nil, fmt.Errorf("kms decryption failed: %w", err)
	}

	checkpoint := &checkpoint.Checkpoint{}
	if err := json.Unmarshal(result.Plaintext, checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return checkpoint, nil
}
