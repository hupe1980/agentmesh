// Package checkpoint provides interfaces and implementations for persisting
// graph execution state.
//
// Checkpointing enables fault-tolerant workflows by saving execution progress
// at regular intervals. If a workflow fails or is interrupted, it can be resumed
// from the last checkpoint without starting over.
//
// # Overview
//
// The package defines a Checkpointer interface that can be implemented with
// various storage backends:
//
//   - MemoryCheckpointer: In-memory storage (testing/development)
//   - SQLCheckpointer: SQL database - SQLite, PostgreSQL, MySQL (production)
//   - DynamoDBCheckpointer: AWS DynamoDB (serverless/cloud production)
//
// See subpackages for specific implementations:
//   - pkg/checkpoint/sql: SQL-based checkpointers (SQLite, PostgreSQL, MySQL)
//   - pkg/checkpoint/dynamodb: AWS DynamoDB checkpointer
//
// # Basic Usage
//
//	// In-memory (testing/development)
//	checkpointer := checkpoint.NewInMemoryCheckpointer()
//
//	// SQL (production - see pkg/checkpoint/sql for details)
//	checkpointer := sql.NewSQLiteCheckpointer("checkpoints.db")
//	// or
//	checkpointer := sql.NewPostgreSQLCheckpointer(connString)
//
//	// DynamoDB (AWS production - see pkg/checkpoint/dynamodb for details)
//	checkpointer := dynamodb.NewCheckpointer(dynamoClient, "checkpoints-table")
//
//	// Configure graph with checkpointer
//	g := graph.New[string, string](keys...)
//	g.WithCheckpointer(checkpointer, "workflow-123")
//	compiled, _ := g.Build()
//
//	// Run with checkpointing
//	for _, err := range compiled.Run(ctx, input,
//	    graph.WithCheckpointInterval(1),  // Save every superstep
//	) {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// # Checkpoint Signing (Security)
//
// Enable HMAC-SHA256 signatures to detect tampering:
//
//	// Generate secure signing key (32+ bytes recommended)
//	signingKey := make([]byte, 32)
//	rand.Read(signingKey)
//
//	// Create checkpointer with signing enabled
//	checkpointer := checkpoint.NewInMemoryCheckpointer(
//	    checkpoint.WithSigning(signingKey),
//	)
//
//	// Checkpoints are automatically signed on Save()
//	// and verified on Load() - fails if tampered
//	checkpoint, err := checkpointer.Load(ctx, "workflow-123")
//	if errors.Is(err, checkpoint.ErrInvalidSignature) {
//	    // Tampering detected!
//	}
//
// See examples/checkpoint_signing for comprehensive usage.
//
// # Resume from Checkpoint
//
//	// Load and inspect checkpoint
//	cp, _ := checkpointer.Load(ctx, "workflow-123")
//	fmt.Printf("Last checkpoint at superstep %d\n", cp.Superstep)
//
//	// Resume execution using the Resume method
//	for _, err := range compiled.Resume(ctx, "workflow-123") {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
//	// Or resume with a specific checkpoint
//	for _, err := range compiled.Resume(ctx, "workflow-123",
//	    graph.WithCheckpoint(cp),
//	) {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// # Time-Travel Debugging
//
//	// Load checkpoint
//	checkpoint, _ := checkpointer.Load(ctx, "workflow-123")
//
//	// Resume from that checkpoint
//	for _, err := range compiled.Resume(ctx, "workflow-123",
//	    graph.WithCheckpoint(checkpoint),
//	) {
//	    // Handle results
//	}
//
// # Checkpoint Structure
//
// Each checkpoint captures:
//   - RunID: Unique workflow identifier
//   - Superstep: BSP superstep number
//   - Timestamp: When checkpoint was created
//   - State: Full graph state including message history (via MessagesKey in state channels)
//   - CompletedNodes: Nodes that finished execution (for smart resume)
//   - PausedNodes: Nodes waiting for input (for human-in-the-loop workflows)
//   - Metadata: Custom annotations
//
// Note: Message history is stored in State (not as a separate field) via the
// MessagesKey channel, enabling consistent state management and restoration.
package checkpoint
