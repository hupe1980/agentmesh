package checkpoint

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

var (
	// ErrInvalidSignature indicates the checkpoint signature verification failed.
	// This could mean the checkpoint was tampered with or signed with a different key.
	ErrInvalidSignature = errors.New("checkpoint: invalid signature")

	// ErrSigningKeyRequired indicates signing was expected but no key was provided.
	ErrSigningKeyRequired = errors.New("checkpoint: signing key required but not configured")

	// ErrEmptySigningKey indicates an empty signing key was provided.
	ErrEmptySigningKey = errors.New("checkpoint: signing key cannot be empty")
)

// SignCheckpoint generates an HMAC-SHA256 signature for a checkpoint using the provided key.
// The signature covers all critical checkpoint fields to detect any tampering.
//
// Signed fields (in order):
//   - RunID
//   - Superstep
//   - Version
//   - Timestamp (Unix nanoseconds)
//   - State (keys and values, sorted by key)
//   - CompletedNodes (sorted)
//   - PausedNodes (sorted)
//
// The Signature field itself is excluded from signing to avoid circular dependency.
// Messages are intentionally excluded as they can be very large and are typically
// rebuilt from state during recovery.
func SignCheckpoint(cp *Checkpoint, key []byte) ([]byte, error) {
	if cp == nil {
		return nil, errors.New("checkpoint is nil")
	}

	if key == nil {
		return nil, ErrSigningKeyRequired
	}

	if len(key) == 0 {
		return nil, ErrEmptySigningKey
	}

	h := hmac.New(sha256.New, key)

	// Sign RunID
	h.Write([]byte(cp.RunID))

	// Sign Superstep (safe conversion: supersteps are always non-negative in practice)
	superstepBytes := make([]byte, 8)
	if cp.Superstep < 0 {
		return nil, fmt.Errorf("invalid negative superstep: %d", cp.Superstep)
	}
	binary.BigEndian.PutUint64(superstepBytes, uint64(cp.Superstep))
	h.Write(superstepBytes)

	// Sign Version
	versionBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(versionBytes, cp.Version)
	h.Write(versionBytes)

	// Sign Timestamp (Unix nanoseconds)
	timestampBytes := make([]byte, 8)
	unixNano := cp.Timestamp.UnixNano()
	if unixNano < 0 {
		return nil, fmt.Errorf("invalid negative timestamp: %d", unixNano)
	}
	binary.BigEndian.PutUint64(timestampBytes, uint64(unixNano))
	h.Write(timestampBytes)

	// Sign State (sorted by key for deterministic signature)
	if cp.State != nil {
		keys := make([]string, 0, len(cp.State))
		for k := range cp.State {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			h.Write([]byte(key))
			// Write value as string representation
			if _, err := fmt.Fprint(h, cp.State[key]); err != nil {
				return nil, fmt.Errorf("failed to write state value: %w", err)
			}
		}
	}

	// Sign CompletedNodes (sorted)
	if len(cp.CompletedNodes) > 0 {
		sortedCompleted := make([]string, len(cp.CompletedNodes))
		copy(sortedCompleted, cp.CompletedNodes)
		sort.Strings(sortedCompleted)
		for _, node := range sortedCompleted {
			h.Write([]byte(node))
		}
	}

	// Sign PausedNodes (sorted)
	if len(cp.PausedNodes) > 0 {
		sortedPaused := make([]string, len(cp.PausedNodes))
		copy(sortedPaused, cp.PausedNodes)
		sort.Strings(sortedPaused)
		for _, node := range sortedPaused {
			h.Write([]byte(node))
		}
	}

	return h.Sum(nil), nil
}

// VerifyCheckpoint verifies a checkpoint's signature using the provided key.
// Returns nil if the signature is valid, ErrInvalidSignature if invalid,
// or another error if verification cannot be performed.
func VerifyCheckpoint(cp *Checkpoint, key []byte) error {
	if cp == nil {
		return errors.New("checkpoint is nil")
	}

	if key == nil {
		return ErrSigningKeyRequired
	}

	if len(key) == 0 {
		return ErrEmptySigningKey
	}

	if len(cp.Signature) == 0 {
		return ErrInvalidSignature
	}

	// Compute expected signature
	expectedSig, err := SignCheckpoint(cp, key)
	if err != nil {
		return fmt.Errorf("failed to compute signature: %w", err)
	}

	// Constant-time comparison to prevent timing attacks
	if !hmac.Equal(cp.Signature, expectedSig) {
		return ErrInvalidSignature
	}

	return nil
}
