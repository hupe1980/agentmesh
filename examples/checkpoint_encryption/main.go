package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"github.com/hupe1980/agentmesh/pkg/agent"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Checkpoint Encryption Example ===")

	// Example 1: Basic AES-256-GCM Encryption
	fmt.Println("1. Basic Encryption with AES-256-GCM")
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

	fmt.Printf("Generated encryption key: %s\n", base64.StdEncoding.EncodeToString(key))

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

	// Define state keys
	creditCardKey := graphstate.NewKey("credit_card", "")
	ssnKey := graphstate.NewKey("ssn", "")
	passwordKey := graphstate.NewKey("password", "")

	// Create a simple graph
	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, agent.MessagesKey.Key)
	graphstate.RegisterKey(mgr, creditCardKey)
	graphstate.RegisterKey(mgr, ssnKey)
	graphstate.RegisterKey(mgr, passwordKey)

	g, err := graph.NewGraph(mgr)
	if err != nil {
		log.Fatal(err)
	}

	g.AddNode(graph.NewBaseNode("secure_node",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			fmt.Println("  Processing sensitive data...")
			return graphstate.Updates{
				creditCardKey.Name(): "4111-1111-1111-1111",
				ssnKey.Name():        "123-45-6789",
				passwordKey.Name():   "super-secret",
			}, nil
		},
	))

	g.AddEdge(graph.StartNode, "secure_node")
	g.AddEdge("secure_node", graph.EndNode)

	// Compile using exec package
	compiled, err := exec.CompileGraph(g, exec.NewPregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	// Run graph (checkpoints will be encrypted)
	messages := []message.Message{
		message.NewHumanMessageFromText("Process secure data"),
	}

	for event, err := range compiled.Run(ctx, messages,
		graph.WithCheckpointer(encryptedCheckpointer),
		graph.WithRunID("encrypted-run"),
	) {
		if err != nil {
			log.Printf("  Error: %v", err)
			continue
		}
		if event != nil {
			fmt.Printf("  Message: %s\n", event.Type())
		}
	}

	// Verify checkpoint is encrypted
	rawCP, err := baseCheckpointer.Load(ctx, "encrypted-run")
	if err == nil && rawCP != nil {
		if encrypted, ok := rawCP.Metadata["encrypted"].(bool); ok && encrypted {
			fmt.Println("  ✓ Checkpoint is encrypted (AES-256-GCM)")
			fmt.Println("  ✓ Sensitive data is protected at rest")
		}
	}
}

func keyRotationExample(ctx context.Context) {
	// Simulate key rotation scenario
	oldKey := []byte("old_key_123456789012345678901234") // 32 bytes
	newKey := []byte("new_key_098765432109876543210987") // 32 bytes

	baseCheckpointer := checkpoint.NewInMemoryCheckpointer()

	// Step 1: Save checkpoint with old key
	fmt.Println("  Step 1: Saving checkpoint with OLD key...")
	oldEncryptor, err := checkpoint.NewAES256GCMEncryptor(oldKey)
	if err != nil {
		log.Fatal(err)
	}
	oldEncrypted, err := checkpoint.NewEncryptedCheckpointer(baseCheckpointer, oldEncryptor)
	if err != nil {
		log.Fatal(err)
	}

	cp := &checkpoint.Checkpoint{
		RunID:     "rotation-test",
		Superstep: 1,
		State: map[string]any{
			"secret": "confidential data",
		},
		Metadata: map[string]any{},
	}
	if err := oldEncrypted.Save(ctx, cp); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  ✓ Checkpoint saved with old key")

	// Step 2: Rotate keys - use MultiKeyCheckpointer
	fmt.Println("  Step 2: Rotating to NEW key (with old key as fallback)...")
	multiKeyCheckpointer, err := checkpoint.NewMultiKeyCheckpointer(
		baseCheckpointer,
		newKey, // Current key for new writes
		oldKey, // Old key(s) for reading legacy checkpoints
	)
	if err != nil {
		log.Fatal(err)
	}

	// Step 3: Load with new checkpointer (will use old key automatically)
	fmt.Println("  Step 3: Loading checkpoint (automatically tries keys)...")
	loadedCP, err := multiKeyCheckpointer.Load(ctx, "rotation-test")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("  ✓ Successfully loaded checkpoint encrypted with old key")

	// Step 4: Re-save with new key
	fmt.Println("  Step 4: Re-saving checkpoint with NEW key...")
	loadedCP.Superstep = 2
	if err := multiKeyCheckpointer.Save(ctx, loadedCP); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  ✓ Checkpoint now encrypted with new key")
	fmt.Println("  ✓ Key rotation complete!")
}

func passwordBasedExample(ctx context.Context) {
	// Derive encryption key from password
	password := os.Getenv("CHECKPOINT_PASSWORD")
	if password == "" {
		password = "demo-password-12345" // For demo only - use env var in production!
	}

	salt := []byte("unique-application-salt")
	key := checkpoint.DeriveKeyFromPassword(password, salt)

	fmt.Printf("  Password: %s\n", password)
	fmt.Printf("  Derived key (first 16 bytes): %x...\n", key[:16])

	baseCheckpointer := checkpoint.NewInMemoryCheckpointer()
	encryptor, err := checkpoint.NewAES256GCMEncryptor(key)
	if err != nil {
		log.Fatal(err)
	}
	encryptedCheckpointer, err := checkpoint.NewEncryptedCheckpointer(baseCheckpointer, encryptor)
	if err != nil {
		log.Fatal(err)
	}

	// Save encrypted checkpoint
	cp := &checkpoint.Checkpoint{
		RunID:     "password-protected",
		Superstep: 1,
		State: map[string]any{
			"data": "password-protected data",
		},
		Metadata: map[string]any{},
	}
	if err := encryptedCheckpointer.Save(ctx, cp); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  ✓ Checkpoint encrypted using password-derived key")

	// Load with same password
	loadedCP, err := encryptedCheckpointer.Load(ctx, "password-protected")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  ✓ Successfully loaded: %v\n", loadedCP.State["data"])

	// Try with wrong password (would fail in real scenario)
	wrongKey := checkpoint.DeriveKeyFromPassword("wrong-password", salt)
	wrongEncryptor, err := checkpoint.NewAES256GCMEncryptor(wrongKey)
	if err != nil {
		log.Fatal(err)
	}
	wrongEncrypted, err := checkpoint.NewEncryptedCheckpointer(baseCheckpointer, wrongEncryptor)
	if err != nil {
		log.Fatal(err)
	}
	_, err = wrongEncrypted.Load(ctx, "password-protected")
	if err != nil {
		fmt.Println("  ✓ Wrong password correctly rejected")
	}
}
