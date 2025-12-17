// Package main demonstrates checkpoint encryption for secure state storage.
//
// This example shows:
//   - AES-256-GCM encryption for checkpoints
//   - Key rotation for security
//   - Password-derived encryption keys
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

// State keys for sensitive data
var (
	creditCardKey = graph.NewKey[string]("credit_card")
	ssnKey        = graph.NewKey[string]("ssn")
	passwordKey   = graph.NewKey[string]("password")
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Checkpoint Encryption Example ===")

	// Example 1: Basic AES-256-GCM Encryption
	fmt.Println("\n1. Basic Encryption with AES-256-GCM")
	basicEncryptionExample(ctx)

	// Example 2: Key Rotation
	fmt.Println("\n2. Key Rotation Example")
	keyRotationExample(ctx)

	// Example 3: Password-Derived Key
	fmt.Println("\n3. Password-Derived Encryption Key")
	passwordBasedExample(ctx)
}

func basicEncryptionExample(ctx context.Context) {
	// Generate a secure 32-byte encryption key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("  Generated encryption key: %s...\n", base64.StdEncoding.EncodeToString(key)[:20])

	// Create base checkpointer (in-memory for demo)
	baseCheckpointer := checkpoint.NewInMemoryCheckpointer()

	// Create encryptor
	encryptor, err := checkpoint.NewAES256GCMEncryptor(key)
	if err != nil {
		log.Fatal(err)
	}

	// Wrap with encryption
	encryptedCheckpointer, err := checkpoint.NewEncryptedCheckpointer(baseCheckpointer, encryptor)
	if err != nil {
		log.Fatal(err)
	}

	// Build graph with sensitive data
	g := graph.New(creditCardKey, ssnKey, passwordKey)

	g.Node("secure_node", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		fmt.Println("  Processing sensitive data...")
		return graph.Set(creditCardKey, "4111-1111-1111-1111").
			With(graph.SetValue(ssnKey, "123-45-6789")).
			With(graph.SetValue(passwordKey, "super-secret")).
			End()
	}, graph.END)

	g.Start("secure_node")
	g.WithCheckpointer(encryptedCheckpointer, "encrypted-run-001")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	// Run - sensitive data is encrypted when checkpointed
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("  ✓ Sensitive data encrypted in checkpoint")

	// Verify we can load it back
	cp, err := encryptedCheckpointer.Load(ctx, "encrypted-run-001")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("  Decrypted state: credit_card=%s, ssn=%s\n",
		cp.State["credit_card"], cp.State["ssn"])
}

func keyRotationExample(ctx context.Context) {
	// Old key
	oldKey := make([]byte, 32)
	rand.Read(oldKey)

	// New key for rotation
	newKey := make([]byte, 32)
	rand.Read(newKey)

	fmt.Println("  Key rotation allows updating encryption keys without data loss")
	fmt.Printf("  Old key: %s...\n", base64.StdEncoding.EncodeToString(oldKey)[:16])
	fmt.Printf("  New key: %s...\n", base64.StdEncoding.EncodeToString(newKey)[:16])

	// Create checkpointer with old key
	baseCheckpointer := checkpoint.NewInMemoryCheckpointer()
	oldEncryptor, _ := checkpoint.NewAES256GCMEncryptor(oldKey)
	encryptedCheckpointer, _ := checkpoint.NewEncryptedCheckpointer(baseCheckpointer, oldEncryptor)

	// Save checkpoint with old key
	cp := &checkpoint.Checkpoint{
		RunID:     "rotation-test",
		Superstep: 1,
		State:     map[string]any{"secret": "my-secret-data"},
	}
	encryptedCheckpointer.Save(ctx, cp)
	fmt.Println("  Saved checkpoint with old key")

	// Load and re-save with new key
	loaded, _ := encryptedCheckpointer.Load(ctx, "rotation-test")
	newEncryptor, _ := checkpoint.NewAES256GCMEncryptor(newKey)
	newCheckpointer, _ := checkpoint.NewEncryptedCheckpointer(baseCheckpointer, newEncryptor)
	newCheckpointer.Save(ctx, loaded)

	fmt.Println("  ✓ Key rotated successfully")
}

func passwordBasedExample(ctx context.Context) {
	password := "my-secure-password"
	salt := make([]byte, 16)
	rand.Read(salt)

	fmt.Println("  Deriving encryption key from password...")
	fmt.Printf("  Password: %s\n", password)

	// Derive key from password using PBKDF2
	key := checkpoint.DeriveKeyFromPassword(password, salt)
	fmt.Printf("  Derived key: %s...\n", base64.StdEncoding.EncodeToString(key)[:20])

	// Use derived key for encryption
	baseCheckpointer := checkpoint.NewInMemoryCheckpointer()
	encryptor, _ := checkpoint.NewAES256GCMEncryptor(key)
	encryptedCheckpointer, _ := checkpoint.NewEncryptedCheckpointer(baseCheckpointer, encryptor)

	// Save encrypted checkpoint
	cp := &checkpoint.Checkpoint{
		RunID:     "password-derived",
		Superstep: 1,
		State:     map[string]any{"sensitive": "password-protected-data"},
	}
	encryptedCheckpointer.Save(ctx, cp)

	// Verify decryption works
	loaded, _ := encryptedCheckpointer.Load(ctx, "password-derived")
	fmt.Printf("  Decrypted: %v\n", loaded.State["sensitive"])
	fmt.Println("  ✓ Password-based encryption working")
}
