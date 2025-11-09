package dynamodb

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// mockClient is a mock implementation of Client for testing.
type mockClient struct {
	mu    sync.RWMutex
	items map[string]map[string]types.AttributeValue // map[runID+superstep]item

	// Tracking for test assertions
	CreateTableCalls int
	PutItemCalls     int
	GetItemCalls     int
	QueryCalls       int
	DeleteItemCalls  int

	// Error injection
	CreateTableError error
	PutItemError     error
	GetItemError     error
	QueryError       error
	DeleteItemError  error
}

// newMockClient creates a new mock DynamoDB client.
func newMockClient() *mockClient {
	return &mockClient{
		items: make(map[string]map[string]types.AttributeValue),
	}
}

func (m *mockClient) CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateTableCalls++

	if m.CreateTableError != nil {
		return nil, m.CreateTableError
	}

	return &dynamodb.CreateTableOutput{}, nil
}

func (m *mockClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.PutItemCalls++

	if m.PutItemError != nil {
		return nil, m.PutItemError
	}

	// Extract run_id and superstep to create key
	runID := params.Item["run_id"].(*types.AttributeValueMemberS).Value
	superstep := params.Item["superstep"].(*types.AttributeValueMemberN).Value
	key := fmt.Sprintf("%s:%s", runID, superstep)

	// Store the item
	m.items[key] = params.Item

	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.GetItemCalls++

	if m.GetItemError != nil {
		return nil, m.GetItemError
	}

	// Extract key
	runID := params.Key["run_id"].(*types.AttributeValueMemberS).Value
	superstep := params.Key["superstep"].(*types.AttributeValueMemberN).Value
	key := fmt.Sprintf("%s:%s", runID, superstep)

	item, exists := m.items[key]
	if !exists {
		return &dynamodb.GetItemOutput{Item: nil}, nil
	}

	return &dynamodb.GetItemOutput{Item: item}, nil
}

func (m *mockClient) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.QueryCalls++

	if m.QueryError != nil {
		return nil, m.QueryError
	}

	// Extract run_id from expression attribute values
	runID := params.ExpressionAttributeValues[":run_id"].(*types.AttributeValueMemberS).Value

	// Find all items with matching run_id
	var matchingItems []map[string]types.AttributeValue
	for _, item := range m.items {
		itemRunID := item["run_id"].(*types.AttributeValueMemberS).Value
		if itemRunID == runID {
			matchingItems = append(matchingItems, item)
		}
	}

	// Sort by superstep (descending if ScanIndexForward is false)
	if params.ScanIndexForward != nil && !*params.ScanIndexForward {
		// Simple descending sort by superstep
		for i := 0; i < len(matchingItems); i++ {
			for j := i + 1; j < len(matchingItems); j++ {
				superstepI := matchingItems[i]["superstep"].(*types.AttributeValueMemberN).Value
				superstepJ := matchingItems[j]["superstep"].(*types.AttributeValueMemberN).Value
				if superstepI < superstepJ {
					matchingItems[i], matchingItems[j] = matchingItems[j], matchingItems[i]
				}
			}
		}
	}

	// Apply limit if specified
	if params.Limit != nil && int(*params.Limit) < len(matchingItems) {
		matchingItems = matchingItems[:*params.Limit]
	}

	return &dynamodb.QueryOutput{
		Items: matchingItems,
		Count: int32(len(matchingItems)),
	}, nil
}

func (m *mockClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteItemCalls++

	if m.DeleteItemError != nil {
		return nil, m.DeleteItemError
	}

	// Extract key
	runID := params.Key["run_id"].(*types.AttributeValueMemberS).Value
	superstep := params.Key["superstep"].(*types.AttributeValueMemberN).Value
	key := fmt.Sprintf("%s:%s", runID, superstep)

	delete(m.items, key)

	return &dynamodb.DeleteItemOutput{}, nil
}

// Helper methods for testing

// reset clears all stored items and counters.
func (m *mockClient) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.items = make(map[string]map[string]types.AttributeValue)
	m.CreateTableCalls = 0
	m.PutItemCalls = 0
	m.GetItemCalls = 0
	m.QueryCalls = 0
	m.DeleteItemCalls = 0
}

// itemCount returns the number of stored items.
func (m *mockClient) itemCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

func TestCheckpointer_Save(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	cp := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Timestamp: time.Now(),
		State: map[string]any{
			"counter": 42,
		},
		Messages:       []message.Message{},
		CompletedNodes: []string{"node1"},
		PausedNodes:    []string{},
		Metadata:       map[string]any{"version": "1.0"},
	}

	err := checkpointer.Save(ctx, cp)
	if err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	if mock.PutItemCalls != 1 {
		t.Errorf("Expected 1 PutItem call, got %d", mock.PutItemCalls)
	}

	if mock.itemCount() != 1 {
		t.Errorf("Expected 1 item in mock, got %d", mock.itemCount())
	}
}

func TestCheckpointer_Load(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	// Save a checkpoint
	original := &checkpoint.Checkpoint{
		RunID:     "test-run",
		Superstep: 1,
		Timestamp: time.Now(),
		State: map[string]any{
			"counter": 42,
			"status":  "running",
		},
		Messages:       []message.Message{},
		CompletedNodes: []string{"node1", "node2"},
		PausedNodes:    []string{},
		Metadata:       map[string]any{"version": "1.0"},
	}

	if err := checkpointer.Save(ctx, original); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Load the checkpoint
	loaded, err := checkpointer.Load(ctx, "test-run")
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if loaded == nil {
		t.Fatal("Loaded checkpoint is nil")
	}

	if loaded.RunID != original.RunID {
		t.Errorf("RunID mismatch: got %s, want %s", loaded.RunID, original.RunID)
	}

	if loaded.Superstep != original.Superstep {
		t.Errorf("Superstep mismatch: got %d, want %d", loaded.Superstep, original.Superstep)
	}

	if mock.QueryCalls != 1 {
		t.Errorf("Expected 1 Query call, got %d", mock.QueryCalls)
	}
}

func TestCheckpointer_LoadNonExistent(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	loaded, err := checkpointer.Load(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if loaded != nil {
		t.Error("Expected nil for non-existent checkpoint")
	}
}

func TestCheckpointer_List(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	// Save multiple checkpoints
	for i := int64(1); i <= 5; i++ {
		cp := &checkpoint.Checkpoint{
			RunID:     "test-run",
			Superstep: i,
			Timestamp: time.Now(),
			State: map[string]any{
				"superstep": i,
			},
			Messages:       []message.Message{},
			CompletedNodes: []string{},
			PausedNodes:    []string{},
			Metadata:       map[string]any{},
		}

		if err := checkpointer.Save(ctx, cp); err != nil {
			t.Fatalf("Failed to save checkpoint %d: %v", i, err)
		}
	}

	// List all checkpoints
	all, err := checkpointer.List(ctx, "test-run")
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(all) != 5 {
		t.Errorf("Expected 5 checkpoints, got %d", len(all))
	}

	// Verify descending order
	for i, cp := range all {
		expectedSuperstep := int64(5 - i)
		if cp.Superstep != expectedSuperstep {
			t.Errorf("Checkpoint %d: expected superstep %d, got %d", i, expectedSuperstep, cp.Superstep)
		}
	}
}

func TestCheckpointer_LoadAtSuperstep(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	// Save multiple checkpoints
	for i := int64(1); i <= 5; i++ {
		cp := &checkpoint.Checkpoint{
			RunID:     "test-run",
			Superstep: i,
			Timestamp: time.Now(),
			State: map[string]any{
				"superstep": i,
			},
			Messages:       []message.Message{},
			CompletedNodes: []string{},
			PausedNodes:    []string{},
			Metadata:       map[string]any{},
		}

		if err := checkpointer.Save(ctx, cp); err != nil {
			t.Fatalf("Failed to save checkpoint %d: %v", i, err)
		}
	}

	// Load specific superstep
	cp3, err := checkpointer.LoadAtSuperstep(ctx, "test-run", 3)
	if err != nil {
		t.Fatalf("Failed to load superstep 3: %v", err)
	}

	if cp3 == nil {
		t.Fatal("Expected checkpoint at superstep 3")
	}

	if cp3.Superstep != 3 {
		t.Errorf("Expected superstep 3, got %d", cp3.Superstep)
	}

	if mock.GetItemCalls != 1 {
		t.Errorf("Expected 1 GetItem call, got %d", mock.GetItemCalls)
	}
}

func TestCheckpointer_Delete(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	// Save checkpoints
	for i := int64(1); i <= 3; i++ {
		cp := &checkpoint.Checkpoint{
			RunID:     "test-run",
			Superstep: i,
			Timestamp: time.Now(),
			State:     map[string]any{},
			Messages:  []message.Message{},
		}

		if err := checkpointer.Save(ctx, cp); err != nil {
			t.Fatalf("Failed to save checkpoint: %v", err)
		}
	}

	if mock.itemCount() != 3 {
		t.Errorf("Expected 3 items before delete, got %d", mock.itemCount())
	}

	// Delete all checkpoints
	if err := checkpointer.Delete(ctx, "test-run"); err != nil {
		t.Fatalf("Failed to delete checkpoints: %v", err)
	}

	if mock.itemCount() != 0 {
		t.Errorf("Expected 0 items after delete, got %d", mock.itemCount())
	}

	// Verify deletion
	loaded, err := checkpointer.Load(ctx, "test-run")
	if err != nil {
		t.Fatalf("Error loading after delete: %v", err)
	}

	if loaded != nil {
		t.Error("Expected nil after deletion")
	}
}

func TestCheckpointer_DeleteNonExistent(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	err := checkpointer.Delete(ctx, "non-existent")
	if err == nil {
		t.Error("Expected error when deleting non-existent run")
	}
}

func TestCheckpointer_SaveNilCheckpoint(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	err := checkpointer.Save(ctx, nil)
	if err == nil {
		t.Error("Expected error when saving nil checkpoint")
	}
}

func TestCheckpointer_SaveEmptyRunID(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	cp := &checkpoint.Checkpoint{
		RunID:     "",
		Superstep: 1,
		Timestamp: time.Now(),
		State:     map[string]any{},
		Messages:  []message.Message{},
	}

	err := checkpointer.Save(ctx, cp)
	if err == nil {
		t.Error("Expected error when saving checkpoint with empty RunID")
	}
}

func TestCheckpointer_LoadMostRecent(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	// Save checkpoints with different supersteps
	for i := int64(1); i <= 5; i++ {
		cp := &checkpoint.Checkpoint{
			RunID:     "test-run",
			Superstep: i,
			Timestamp: time.Now(),
			State: map[string]any{
				"counter": i * 10,
			},
			Messages: []message.Message{},
		}

		if err := checkpointer.Save(ctx, cp); err != nil {
			t.Fatalf("Failed to save checkpoint: %v", err)
		}
	}

	// Load should return most recent (superstep 5)
	loaded, err := checkpointer.Load(ctx, "test-run")
	if err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	if loaded.Superstep != 5 {
		t.Errorf("Expected superstep 5, got %d", loaded.Superstep)
	}

	state := loaded.State["counter"]
	if state != float64(50) { // JSON unmarshals numbers as float64
		t.Errorf("Expected counter 50, got %v", state)
	}
}

func TestCheckpointer_CreateTable(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock)
	ctx := context.Background()

	err := checkpointer.CreateTable(ctx)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	if mock.CreateTableCalls != 1 {
		t.Errorf("Expected 1 CreateTable call, got %d", mock.CreateTableCalls)
	}
}

func TestCheckpointer_CustomTableName(t *testing.T) {
	mock := newMockClient()
	checkpointer := NewCheckpointer(mock, WithTableName("custom-checkpoints"))

	if checkpointer.tableName != "custom-checkpoints" {
		t.Errorf("Expected table name 'custom-checkpoints', got '%s'", checkpointer.tableName)
	}
}
