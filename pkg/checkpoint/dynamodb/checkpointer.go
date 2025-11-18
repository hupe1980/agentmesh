package dynamodb

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Client abstracts AWS DynamoDB operations for easier testing and flexibility.
type Client interface {
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

// Ensure *dynamodb.Client from AWS SDK implements our Client interface
var _ Client = (*dynamodb.Client)(nil)

// Checkpointer implements checkpoint.Checkpointer using AWS DynamoDB.
type Checkpointer struct {
	client    Client
	tableName string
}

// Option configures Checkpointer.
type Option func(*Checkpointer)

// WithTableName sets a custom table name (default: "agentmesh-checkpoints").
func WithTableName(name string) Option {
	return func(c *Checkpointer) {
		c.tableName = name
	}
}

// checkpointItem represents the DynamoDB item structure.
// Note: Message history is stored in the State field via MessagesKey, not as a separate Messages field.
type checkpointItem struct {
	RunID          string `dynamodbav:"run_id"`
	Superstep      int64  `dynamodbav:"superstep"`
	Timestamp      string `dynamodbav:"timestamp"`
	State          string `dynamodbav:"state"`
	CompletedNodes string `dynamodbav:"completed_nodes"`
	PausedNodes    string `dynamodbav:"paused_nodes"`
	Metadata       string `dynamodbav:"metadata"`
	TTL            *int64 `dynamodbav:"ttl,omitempty"`
}

// NewCheckpointer creates a new DynamoDB-based checkpointer.
// It does NOT automatically create the table - you must create it manually or use CreateTable.
//
// Example:
//
//	cfg, _ := config.LoadDefaultConfig(ctx)
//	client := dynamodb.NewFromConfig(cfg)
//	checkpointer := dynamodb.NewCheckpointer(client)
func NewCheckpointer(client Client, opts ...Option) *Checkpointer {
	c := &Checkpointer{
		client:    client,
		tableName: "agentmesh-checkpoints",
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// CreateTable creates the DynamoDB table with the required schema.
// This is a convenience method - you can also create the table manually.
//
// Schema:
//   - Partition Key: run_id (String)
//   - Sort Key: superstep (Number)
//   - Billing Mode: PAY_PER_REQUEST (on-demand)
func (c *Checkpointer) CreateTable(ctx context.Context) error {
	_, err := c.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(c.tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("run_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("superstep"),
				AttributeType: types.ScalarAttributeTypeN,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("run_id"),
				KeyType:       types.KeyTypeHash, // Partition key
			},
			{
				AttributeName: aws.String("superstep"),
				KeyType:       types.KeyTypeRange, // Sort key
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})

	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

// Save persists a checkpoint to DynamoDB.
func (c *Checkpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	if cp == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	if cp.RunID == "" {
		return fmt.Errorf("checkpoint RunID is empty")
	}

	// Serialize complex fields to JSON
	stateJSON, err := json.Marshal(cp.State)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	completedNodesJSON, err := json.Marshal(cp.CompletedNodes)
	if err != nil {
		return fmt.Errorf("failed to marshal completed nodes: %w", err)
	}

	pausedNodesJSON, err := json.Marshal(cp.PausedNodes)
	if err != nil {
		return fmt.Errorf("failed to marshal paused nodes: %w", err)
	}

	metadataJSON, err := json.Marshal(cp.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	item := checkpointItem{
		RunID:          cp.RunID,
		Superstep:      cp.Superstep,
		Timestamp:      cp.Timestamp.Format(time.RFC3339Nano),
		State:          string(stateJSON),
		CompletedNodes: string(completedNodesJSON),
		PausedNodes:    string(pausedNodesJSON),
		Metadata:       string(metadataJSON),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	_, err = c.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(c.tableName),
		Item:      av,
	})

	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return nil
}

// Load retrieves the most recent checkpoint for a run ID.
func (c *Checkpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is empty")
	}

	// Query for all checkpoints with this runID, sorted by superstep descending
	result, err := c.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(c.tableName),
		KeyConditionExpression: aws.String("run_id = :run_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":run_id": &types.AttributeValueMemberS{Value: runID},
		},
		ScanIndexForward: aws.Bool(false), // Descending order
		Limit:            aws.Int32(1),    // Only get the most recent
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query checkpoint: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil // No checkpoint found
	}

	return c.unmarshalCheckpoint(result.Items[0])
}

// List returns all checkpoints for a run ID, ordered by superstep (newest first).
func (c *Checkpointer) List(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is empty")
	}

	result, err := c.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(c.tableName),
		KeyConditionExpression: aws.String("run_id = :run_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":run_id": &types.AttributeValueMemberS{Value: runID},
		},
		ScanIndexForward: aws.Bool(false), // Descending order
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoints: %w", err)
	}

	checkpoints := make([]*checkpoint.Checkpoint, 0, len(result.Items))
	for _, item := range result.Items {
		cp, err := c.unmarshalCheckpoint(item)
		if err != nil {
			return nil, err
		}
		checkpoints = append(checkpoints, cp)
	}

	return checkpoints, nil
}

// Delete removes all checkpoints for a run ID.
func (c *Checkpointer) Delete(ctx context.Context, runID string) error {
	if runID == "" {
		return fmt.Errorf("runID is empty")
	}

	// First, query to get all items to delete
	result, err := c.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(c.tableName),
		KeyConditionExpression: aws.String("run_id = :run_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":run_id": &types.AttributeValueMemberS{Value: runID},
		},
		ProjectionExpression: aws.String("run_id, superstep"),
	})

	if err != nil {
		return fmt.Errorf("failed to query checkpoints: %w", err)
	}

	if len(result.Items) == 0 {
		return fmt.Errorf("no checkpoints found for runID: %s", runID)
	}

	// Delete each item (DynamoDB doesn't have batch delete by partition key)
	for _, item := range result.Items {
		_, err := c.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(c.tableName),
			Key:       item,
		})
		if err != nil {
			return fmt.Errorf("failed to delete checkpoint: %w", err)
		}
	}

	return nil
}

// LoadAtSuperstep retrieves a checkpoint at a specific superstep.
func (c *Checkpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is empty")
	}

	result, err := c.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(c.tableName),
		Key: map[string]types.AttributeValue{
			"run_id":    &types.AttributeValueMemberS{Value: runID},
			"superstep": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", superstep)},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get checkpoint: %w", err)
	}

	if result.Item == nil {
		return nil, nil // No checkpoint found at this superstep
	}

	return c.unmarshalCheckpoint(result.Item)
}

// unmarshalCheckpoint converts a DynamoDB item to a Checkpoint.
func (c *Checkpointer) unmarshalCheckpoint(item map[string]types.AttributeValue) (*checkpoint.Checkpoint, error) {
	var dbItem checkpointItem
	if err := attributevalue.UnmarshalMap(item, &dbItem); err != nil {
		return nil, fmt.Errorf("failed to unmarshal item: %w", err)
	}

	cp := &checkpoint.Checkpoint{
		RunID:     dbItem.RunID,
		Superstep: dbItem.Superstep,
	}

	// Parse timestamp
	timestamp, err := time.Parse(time.RFC3339Nano, dbItem.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	cp.Timestamp = timestamp

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(dbItem.State), &cp.State); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	if err := json.Unmarshal([]byte(dbItem.CompletedNodes), &cp.CompletedNodes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal completed nodes: %w", err)
	}

	if err := json.Unmarshal([]byte(dbItem.PausedNodes), &cp.PausedNodes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal paused nodes: %w", err)
	}

	if err := json.Unmarshal([]byte(dbItem.Metadata), &cp.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return cp, nil
}

// Close is a no-op for DynamoDB (no connection to close).
func (c *Checkpointer) Close() error {
	return nil
}
