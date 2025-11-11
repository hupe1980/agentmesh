// Package vault provides HashiCorp Vault transit encryption support for checkpoints.
//
// This package wraps any checkpoint.Checkpointer implementation with Vault's
// transit secrets engine, enabling centralized encryption key management and
// policy enforcement.
//
// # Features
//
//   - Transit secrets engine encryption/decryption
//   - Centralized key management
//   - Key versioning and rotation
//   - Fine-grained access policies
//   - HSM backing support (Vault Enterprise)
//   - Audit logging of all encryption operations
//
// # Usage
//
// Basic usage with Vault transit encryption:
//
//	import (
//	    vault "github.com/hashicorp/vault/api"
//	    "github.com/hupe1980/agentmesh/pkg/checkpoint"
//	    "github.com/hupe1980/agentmesh/pkg/checkpoint/vault"
//	)
//
//	// Create Vault client
//	config := vault.DefaultConfig()
//	config.Address = "https://vault.example.com:8200"
//
//	client, err := vault.NewClient(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	client.SetToken(os.Getenv("VAULT_TOKEN"))
//
//	// Create base checkpointer
//	base := checkpoint.NewSQLiteCheckpointer("./checkpoints.db")
//
//	// Wrap with Vault encryption
//	vaultCP, err := vaultpkg.NewVaultCheckpointer(
//	    base,
//	    client,
//	    "transit",              // mount path
//	    "agentmesh-checkpoints", // key name
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Use in your graph
//	compiled, err := graph.Compile(
//	    graph.WithCheckpointer(vaultCP),
//	)
//
// # Transit Engine Setup
//
// Before using the Vault checkpointer, you need to:
//
//  1. Enable the transit secrets engine:
//     $ vault secrets enable transit
//
//  2. Create an encryption key:
//     $ vault write -f transit/keys/agentmesh-checkpoints
//
//  3. Create a policy for the application:
//     path "transit/encrypt/agentmesh-checkpoints" {
//     capabilities = ["update"]
//     }
//     path "transit/decrypt/agentmesh-checkpoints" {
//     capabilities = ["update"]
//     }
//
//  4. Generate a token with the policy:
//     $ vault token create -policy=agentmesh-policy
//
// # Key Management
//
// The Vault transit engine provides several key management features:
//
//   - Automatic key rotation
//   - Key versioning (automatically handled)
//   - Key derivation
//   - Convergent encryption (optional)
//   - HSM backing (Enterprise)
//
// # Security Considerations
//
//   - Use TLS for all Vault connections (HTTPS)
//   - Rotate Vault tokens according to your security policy
//   - Use limited-scope policies (principle of least privilege)
//   - Enable audit logging in Vault
//   - Consider using AppRole or other machine authentication methods
//   - Never commit Vault tokens to version control
//
// # Performance
//
// Each checkpoint Save/Load operation makes one Vault API call. For high-throughput
// applications, consider:
//
//   - Using Vault's performance standbys
//   - Caching decrypted checkpoints when appropriate
//   - Monitoring Vault API response times
//   - Using local Vault agents for caching
//
// # Backwards Compatibility
//
// The Vault checkpointer gracefully handles unencrypted checkpoints. If a checkpoint
// doesn't have the "encrypted_vault" metadata flag, it's returned as-is without
// attempting decryption.
//
// # Alternative Authentication Methods
//
// While this package uses token authentication, you can configure the Vault client
// with other authentication methods before passing it to NewVaultCheckpointer:
//
//   - AppRole
//   - Kubernetes auth
//   - AWS auth
//   - Azure auth
//   - TLS certificates
//
// See the Vault API documentation for details on these authentication methods.
package vault
