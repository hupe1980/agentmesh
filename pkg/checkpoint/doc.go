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
//	checkpointer := checkpoint.NewMemoryCheckpointer()
//
//	// SQL (production - see pkg/checkpoint/sql for details)
//	checkpointer := sql.NewSQLiteCheckpointer("checkpoints.db")
//	// or
//	checkpointer := sql.NewPostgreSQLCheckpointer(connString)
//
//	// DynamoDB (AWS production - see pkg/checkpoint/dynamodb for details)
//	checkpointer := dynamodb.NewCheckpointer(dynamoClient, "checkpoints-table")
//
//	compiled.Invoke(ctx, messages,
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithRunID("workflow-123"),
//	    graph.WithCheckpointConfig(checkpoint.Config{
//	        SaveInterval: 2,  // Save every 2 supersteps
//	        AutoRestore:  true,
//	    }),
//	)
//
// # Resume from Checkpoint
//
//	// Load and inspect checkpoint
//	checkpoint, _ := checkpointer.Load(ctx, "workflow-123")
//	fmt.Printf("Last checkpoint at superstep %d\n", checkpoint.Superstep)
//
//	// Resume execution
//	compiled.Invoke(ctx, nil,
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithRunID("workflow-123"),
//	    graph.WithCheckpointConfig(checkpoint.Config{
//	        AutoRestore: true,
//	    }),
//	)
//
// # Time-Travel Debugging
//
//	// Load checkpoint from specific superstep
//	checkpoint, _ := checkpointer.LoadAtSuperstep(ctx, "workflow-123", 4)
//
//	// Resume from that point
//	compiled.Invoke(ctx, nil,
//	    graph.WithCheckpointer(checkpointer),
//	    graph.WithRunID("workflow-123"),
//	    graph.WithResumeFromSuperstep(4),
//	)
//
// # Checkpoint Structure
//
// Each checkpoint captures:
//   - RunID: Unique workflow identifier
//   - Superstep: BSP superstep number
//   - Timestamp: When checkpoint was created
//   - State: Full graph state (all channels)
//   - Messages: Message history
//   - CompletedNodes: Executed nodes
//   - PausedNodes: Paused nodes (human-in-loop)
//   - Metadata: Custom annotations
package checkpoint
