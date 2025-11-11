// Package awskms provides AWS KMS encryption support for checkpoints.
//
// This package wraps any checkpoint.Checkpointer implementation with AWS KMS
// encryption, allowing secure storage of checkpoint data using AWS Key Management
// Service.
//
// # Features
//
//   - Server-side encryption using AWS KMS
//   - Automatic key management through AWS
//   - Audit trail via AWS CloudTrail
//   - Fine-grained IAM permissions
//   - Support for customer-managed keys (CMK)
//
// # Usage
//
// Basic usage with AWS KMS encryption:
//
//	import (
//	    "github.com/aws/aws-sdk-go-v2/config"
//	    "github.com/aws/aws-sdk-go-v2/service/kms"
//	    "github.com/hupe1980/agentmesh/pkg/checkpoint"
//	    "github.com/hupe1980/agentmesh/pkg/checkpoint/awskms"
//	)
//
//	// Load AWS configuration
//	cfg, err := config.LoadDefaultConfig(context.Background())
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create KMS client
//	kmsClient := kms.NewFromConfig(cfg)
//
//	// Create base checkpointer
//	base := checkpoint.NewSQLiteCheckpointer("./checkpoints.db")
//
//	// Wrap with KMS encryption
//	kmsCP, err := awskms.NewKMSCheckpointer(
//	    base,
//	    kmsClient,
//	    "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Use in your graph
//	compiled, err := graph.Compile(
//	    graph.WithCheckpointer(kmsCP),
//	)
//
// # Key Management
//
// The KMS checkpointer requires a valid KMS key ID or ARN. You can use:
//
//   - Key ID: "12345678-1234-1234-1234-123456789012"
//   - Key ARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"
//   - Alias: "alias/my-checkpoint-key"
//   - Alias ARN: "arn:aws:kms:us-east-1:123456789012:alias/my-checkpoint-key"
//
// # Security Considerations
//
//   - Ensure the IAM role/user has kms:Encrypt and kms:Decrypt permissions
//   - Use key policies to control access to the KMS key
//   - Enable CloudTrail logging for audit compliance
//   - Consider using different keys for different environments
//   - Rotate KMS keys according to your security policy
//
// # Performance
//
// Each checkpoint Save/Load operation makes one KMS API call. For high-throughput
// applications, consider:
//
//   - Using envelope encryption (encrypt data keys with KMS)
//   - Caching decrypted checkpoints when appropriate
//   - Monitoring KMS API throttling limits
//
// # Backwards Compatibility
//
// The KMS checkpointer gracefully handles unencrypted checkpoints. If a checkpoint
// doesn't have the "encrypted_kms" metadata flag, it's returned as-is without
// attempting decryption.
package awskms
