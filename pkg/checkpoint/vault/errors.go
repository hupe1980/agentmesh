// Package vault provides sentinel errors for the Vault checkpoint package.
package vault

import "github.com/hupe1980/agentmesh/pkg/checkpoint"

var (
	// ErrClientRequired is an alias for checkpoint.ErrVaultClientRequired.
	ErrClientRequired = checkpoint.ErrVaultClientRequired

	// ErrKeyNameRequired is an alias for checkpoint.ErrVaultKeyNameRequired.
	ErrKeyNameRequired = checkpoint.ErrVaultKeyNameRequired

	// ErrMissingCiphertext is an alias for checkpoint.ErrVaultMissingCiphertext.
	ErrMissingCiphertext = checkpoint.ErrVaultMissingCiphertext

	// ErrMissingPlaintext is an alias for checkpoint.ErrVaultMissingPlaintext.
	ErrMissingPlaintext = checkpoint.ErrVaultMissingPlaintext
)
