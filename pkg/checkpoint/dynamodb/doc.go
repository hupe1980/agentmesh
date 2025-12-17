// Package dynamodb provides DynamoDB-based checkpoint persistence using AWS SDK for Go v2.
//
// The dynamodb package implements the checkpoint.Checkpointer interface for AWS DynamoDB,
// providing serverless, highly scalable checkpoint storage for distributed agent workflows.
//
// # Basic Usage
//
//	import (
//	    "github.com/aws/aws-sdk-go-v2/config"
//	    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
//	    dyncp "github.com/hupe1980/agentmesh/pkg/checkpoint/dynamodb"
//	)
//
//	cfg, _ := config.LoadDefaultConfig(ctx)
//	client := dynamodb.NewFromConfig(cfg)
//	checkpointer := dyncp.NewCheckpointer(client)
//
//	// Use with a graph
//	g := graph.New(keys...)
//	compiled, _ := g.Build(graph.WithCheckpointer(checkpointer))
//
// # Table Creation
//
// Create the DynamoDB table with the required schema:
//
//	checkpointer := dyncp.NewCheckpointer(client)
//	err := checkpointer.CreateTable(ctx)
//
// Or create manually with AWS CLI:
//
//	aws dynamodb create-table \
//	    --table-name agentmesh-checkpoints \
//	    --attribute-definitions \
//	        AttributeName=run_id,AttributeType=S \
//	        AttributeName=superstep,AttributeType=N \
//	    --key-schema \
//	        AttributeName=run_id,KeyType=HASH \
//	        AttributeName=superstep,KeyType=RANGE \
//	    --billing-mode PAY_PER_REQUEST
//
// # Table Schema
//
// The checkpointer uses the following schema:
//   - Partition Key: run_id (String) - Graph execution run identifier
//   - Sort Key: superstep (Number) - Graph computation step number
//   - Attributes:
//   - timestamp (String) - ISO 8601 timestamp
//   - state (String) - Serialized graph state (JSON)
//   - messages (String) - Serialized message queue (JSON)
//   - completed_nodes (String) - Array of completed node names (JSON)
//   - paused_nodes (String) - Array of paused node names (JSON)
//   - metadata (String) - Additional checkpoint metadata (JSON)
//   - ttl (Number, optional) - Time-to-live for automatic cleanup
//
// # Features
//
// Serverless and Scalable:
//   - No servers to manage
//   - Automatically scales to handle workload
//   - Pay only for what you use (PAY_PER_REQUEST mode)
//
// High Availability:
//   - Multi-AZ replication by default
//   - 99.99% availability SLA
//   - Automatic backup and point-in-time recovery
//
// Performance:
//   - Single-digit millisecond latency
//   - Efficient queries using partition and sort keys
//   - Optimized for time-travel queries (LoadAtSuperstep)
//
// # Configuration
//
// Custom table name:
//
//	checkpointer := dyncp.NewCheckpointer(client,
//	    dyncp.WithTableName("my-checkpoints"),
//	)
//
// AWS credentials (uses standard AWS SDK configuration):
//
//	// Environment variables
//	export AWS_ACCESS_KEY_ID=your_access_key
//	export AWS_SECRET_ACCESS_KEY=your_secret_key
//	export AWS_REGION=us-east-1
//
//	// Or use AWS profiles
//	cfg, _ := config.LoadDefaultConfig(ctx,
//	    config.WithRegion("us-east-1"),
//	    config.WithSharedConfigProfile("myprofile"),
//	)
//
// # Cost Optimization
//
// Enable TTL for automatic cleanup:
//
//	aws dynamodb update-time-to-live \
//	    --table-name agentmesh-checkpoints \
//	    --time-to-live-specification \
//	        Enabled=true,AttributeName=ttl
//
// Use on-demand billing for unpredictable workloads, or provisioned capacity for steady traffic.
//
// # Use Cases
//
// Ideal for:
//   - Serverless agent deployments (AWS Lambda, ECS Fargate)
//   - Multi-region distributed systems
//   - Workloads with unpredictable traffic patterns
//   - Applications requiring high availability
//   - Systems with automatic scaling requirements
//
// Not recommended for:
//   - High-frequency checkpointing (> 1000 writes/sec sustained)
//   - Very large checkpoint payloads (> 400 KB per checkpoint)
//   - Cost-sensitive workloads with predictable patterns (consider SQL)
//   - Development/testing (consider SQLite or Memory)
//
// # IAM Permissions
//
// Required IAM permissions:
//
//	{
//	    "Version": "2012-10-17",
//	    "Statement": [
//	        {
//	            "Effect": "Allow",
//	            "Action": [
//	                "dynamodb:PutItem",
//	                "dynamodb:GetItem",
//	                "dynamodb:Query",
//	                "dynamodb:DeleteItem",
//	                "dynamodb:CreateTable"
//	            ],
//	            "Resource": "arn:aws:dynamodb:*:*:table/agentmesh-checkpoints"
//	        }
//	    ]
//	}
//
// # Examples
//
// Save and load checkpoints:
//
//	checkpointer := dyncp.NewCheckpointer(client)
//
//	// Save
//	err := checkpointer.Save(ctx, &checkpoint.Checkpoint{
//	    RunID:     "run-123",
//	    Superstep: 1,
//	    Timestamp: time.Now(),
//	    State:     map[string]any{"data": "value"},
//	})
//
//	// Load most recent
//	cp, _ := checkpointer.Load(ctx, "run-123")
//
//	// List all checkpoints
//	all, _ := checkpointer.List(ctx, "run-123")
//
//	// Time travel
//	cp3, _ := checkpointer.LoadAtSuperstep(ctx, "run-123", 3)
//
//	// Delete
//	err = checkpointer.Delete(ctx, "run-123")
//
// # Monitoring
//
// Monitor DynamoDB metrics in CloudWatch:
//   - ConsumedReadCapacityUnits / ConsumedWriteCapacityUnits
//   - SuccessfulRequestLatency
//   - UserErrors / SystemErrors
//   - ThrottledRequests (adjust capacity if seen)
//
// # See Also
//
//   - AWS DynamoDB Documentation: https://docs.aws.amazon.com/dynamodb/
//   - AWS SDK for Go v2: https://aws.github.io/aws-sdk-go-v2/
//   - SQL Checkpointer: github.com/hupe1980/agentmesh/pkg/checkpoint/sql
//   - Memory Checkpointer: github.com/hupe1980/agentmesh/pkg/checkpoint
package dynamodb
