// Package awskms provides sentinel errors for the AWS KMS checkpoint package.
package awskms

import "github.com/hupe1980/agentmesh/pkg/checkpoint"

var (
	// ErrKeyIDRequired is an alias for checkpoint.ErrKMSKeyIDRequired.
	ErrKeyIDRequired = checkpoint.ErrKMSKeyIDRequired

	// ErrClientRequired is an alias for checkpoint.ErrKMSClientRequired.
	ErrClientRequired = checkpoint.ErrKMSClientRequired

	// ErrMissingPayload is an alias for checkpoint.ErrMissingPayload.
	ErrMissingPayload = checkpoint.ErrMissingPayload
)
