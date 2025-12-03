package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/pregel"
	predis "github.com/hupe1980/agentmesh/pkg/pregel/redis"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// TestRedisMessageBus_BasicOperations tests core message bus operations
func TestRedisMessageBus_BasicOperations(t *testing.T) {
	ctx := context.Background()

	// Start Redis container
	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get Redis endpoint
	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	// Create message bus
	bus, err := predis.NewMessageBus[string](addr, "", 0, &predis.Options{
		Namespace: "test-basic",
		TTL:       1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus.Close()

	// Test Ping
	if err := bus.Ping(ctx); err != nil {
		t.Fatalf("Failed to ping Redis: %v", err)
	}

	// Test Send
	messages := []pregel.Message[string]{
		{To: "node1", Data: "message1"},
		{To: "node1", Data: "message2"},
		{To: "node2", Data: "message3"},
	}

	if err := bus.Send(ctx, messages); err != nil {
		t.Fatalf("Failed to send messages: %v", err)
	}

	// Test Receive
	received1, err := bus.Receive(ctx, "node1")
	if err != nil {
		t.Fatalf("Failed to receive from node1: %v", err)
	}

	if len(received1) != 2 {
		t.Errorf("Expected 2 messages for node1, got %d", len(received1))
	}

	// Verify FIFO order (LPUSH + RPOP)
	if received1[0].Data != "message1" {
		t.Errorf("Expected first message to be 'message1', got %q", received1[0].Data)
	}
	if received1[1].Data != "message2" {
		t.Errorf("Expected second message to be 'message2', got %q", received1[1].Data)
	}

	received2, err := bus.Receive(ctx, "node2")
	if err != nil {
		t.Fatalf("Failed to receive from node2: %v", err)
	}

	if len(received2) != 1 {
		t.Errorf("Expected 1 message for node2, got %d", len(received2))
	}

	// Test Receive on empty mailbox
	empty, err := bus.Receive(ctx, "node1")
	if err != nil {
		t.Fatalf("Failed to receive from empty mailbox: %v", err)
	}

	if empty != nil {
		t.Errorf("Expected nil for empty mailbox, got %d messages", len(empty))
	}

	// Test Clear
	if err := bus.Send(ctx, []pregel.Message[string]{{To: "node3", Data: "test"}}); err != nil {
		t.Fatalf("Failed to send test message: %v", err)
	}

	if err := bus.Clear(ctx, "node3"); err != nil {
		t.Fatalf("Failed to clear node3: %v", err)
	}

	cleared, err := bus.Receive(ctx, "node3")
	if err != nil {
		t.Fatalf("Failed to receive after clear: %v", err)
	}

	if cleared != nil {
		t.Errorf("Expected nil after clear, got %d messages", len(cleared))
	}
}

// TestRedisMessageBus_ConcurrentAccess tests thread-safety
func TestRedisMessageBus_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	bus, err := predis.NewMessageBus[int](addr, "", 0, &predis.Options{
		Namespace: "test-concurrent",
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus.Close()

	// Concurrent senders
	const numSenders = 10
	const messagesPerSender = 100
	var wg sync.WaitGroup

	// Send messages concurrently
	for i := 0; i < numSenders; i++ {
		wg.Add(1)
		go func(senderID int) {
			defer wg.Done()

			for j := 0; j < messagesPerSender; j++ {
				target := fmt.Sprintf("node%d", j%5) // 5 target nodes
				msg := pregel.Message[int]{
					To:   target,
					Data: senderID*1000 + j,
				}

				if err := bus.Send(ctx, []pregel.Message[int]{msg}); err != nil {
					t.Errorf("Sender %d failed to send: %v", senderID, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all messages received
	totalReceived := 0
	for i := 0; i < 5; i++ {
		node := fmt.Sprintf("node%d", i)
		msgs, err := bus.Receive(ctx, node)
		if err != nil {
			t.Errorf("Failed to receive from %s: %v", node, err)
			continue
		}
		totalReceived += len(msgs)
	}

	expected := numSenders * messagesPerSender
	if totalReceived != expected {
		t.Errorf("Expected %d total messages, got %d", expected, totalReceived)
	}
}

// TestRedisMessageBus_Persistence tests message persistence across connections
func TestRedisMessageBus_Persistence(t *testing.T) {
	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	namespace := "test-persistence"

	// Create first bus, send messages, then close
	bus1, err := predis.NewMessageBus[string](addr, "", 0, &predis.Options{
		Namespace: namespace,
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}

	messages := []pregel.Message[string]{
		{To: "vertex1", Data: "persistent1"},
		{To: "vertex1", Data: "persistent2"},
		{To: "vertex2", Data: "persistent3"},
	}

	if err := bus1.Send(ctx, messages); err != nil {
		t.Fatalf("Failed to send messages: %v", err)
	}

	bus1.Close()

	// Create second bus with same namespace, verify messages still there
	bus2, err := predis.NewMessageBus[string](addr, "", 0, &predis.Options{
		Namespace: namespace,
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus2.Close()

	// Verify messages are still there after reconnect
	received, err := bus2.Receive(ctx, "vertex1")
	if err != nil {
		t.Fatalf("Failed to receive: %v", err)
	}

	if len(received) != 2 {
		t.Errorf("Expected 2 persisted messages, got %d", len(received))
	}
}

// TestRedisMessageBus_NamespaceIsolation tests namespace isolation
func TestRedisMessageBus_NamespaceIsolation(t *testing.T) {
	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	// Create two buses with different namespaces
	bus1, err := predis.NewMessageBus[string](addr, "", 0, &predis.Options{
		Namespace: "namespace1",
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus1.Close()

	bus2, err := predis.NewMessageBus[string](addr, "", 0, &predis.Options{
		Namespace: "namespace2",
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus2.Close()

	// Send messages to bus1
	if err := bus1.Send(ctx, []pregel.Message[string]{
		{To: "node1", Data: "ns1-message"},
	}); err != nil {
		t.Fatalf("Failed to send to bus1: %v", err)
	}

	// Send messages to bus2
	if err := bus2.Send(ctx, []pregel.Message[string]{
		{To: "node1", Data: "ns2-message"},
	}); err != nil {
		t.Fatalf("Failed to send to bus2: %v", err)
	}

	// Verify bus1 only sees its messages
	msgs1, err := bus1.Receive(ctx, "node1")
	if err != nil {
		t.Fatalf("Failed to receive from bus1: %v", err)
	}
	if len(msgs1) != 1 || msgs1[0].Data != "ns1-message" {
		t.Errorf("Bus1 received wrong messages: %v", msgs1)
	}

	// Verify bus2 only sees its messages
	msgs2, err := bus2.Receive(ctx, "node1")
	if err != nil {
		t.Fatalf("Failed to receive from bus2: %v", err)
	}
	if len(msgs2) != 1 || msgs2[0].Data != "ns2-message" {
		t.Errorf("Bus2 received wrong messages: %v", msgs2)
	}
}

// TestRedisMessageBus_CleanNamespace tests namespace cleanup
func TestRedisMessageBus_CleanNamespace(t *testing.T) {
	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	bus, err := predis.NewMessageBus[string](addr, "", 0, &predis.Options{
		Namespace: "test-clean",
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus.Close()

	// Send messages
	messages := []pregel.Message[string]{
		{To: "node1", Data: "msg1"},
		{To: "node2", Data: "msg2"},
		{To: "node3", Data: "msg3"},
	}

	if err := bus.Send(ctx, messages); err != nil {
		t.Fatalf("Failed to send messages: %v", err)
	}

	// Verify messages exist
	stats, err := bus.Stats(ctx)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.TotalMessages != 3 {
		t.Errorf("Expected 3 messages before cleanup, got %d", stats.TotalMessages)
	}

	// Clean namespace
	if err := bus.CleanNamespace(ctx); err != nil {
		t.Fatalf("Failed to clean namespace: %v", err)
	}

	// Verify all messages gone
	stats, err = bus.Stats(ctx)
	if err != nil {
		t.Fatalf("Failed to get stats after cleanup: %v", err)
	}

	if stats.TotalMessages != 0 {
		t.Errorf("Expected 0 messages after cleanup, got %d", stats.TotalMessages)
	}
}

// TestRedisMessageBus_Stats tests statistics gathering
func TestRedisMessageBus_Stats(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer container.Terminate(context.Background())

	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	bus, err := predis.NewMessageBus[string](addr, "", 0, &predis.Options{
		Namespace: "test-stats",
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus.Close()

	// Send varying numbers of messages to different vertices
	messages := []pregel.Message[string]{
		{To: "v1", Data: "msg1"},
		{To: "v1", Data: "msg2"},
		{To: "v1", Data: "msg3"},
		{To: "v2", Data: "msg4"},
		{To: "v2", Data: "msg5"},
		{To: "v3", Data: "msg6"},
	}

	if err := bus.Send(ctx, messages); err != nil {
		t.Fatalf("Failed to send messages: %v", err)
	}

	stats, err := bus.Stats(ctx)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.TotalMessages != 6 {
		t.Errorf("Expected 6 total messages, got %d", stats.TotalMessages)
	}

	if stats.VerticesWithMessages != 3 {
		t.Errorf("Expected 3 vertices with messages, got %d", stats.VerticesWithMessages)
	}

	if stats.LargestMailbox != 3 {
		t.Errorf("Expected largest mailbox to be 3, got %d", stats.LargestMailbox)
	}
}

// TestRedisMessageBus_EmptyOperations tests edge cases with empty data
func TestRedisMessageBus_EmptyOperations(t *testing.T) {
	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	bus, err := predis.NewMessageBus[string](addr, "", 0, &predis.Options{
		Namespace: "test-empty",
	})
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}
	defer bus.Close()

	// Send empty message slice
	if err := bus.Send(ctx, []pregel.Message[string]{}); err != nil {
		t.Errorf("Send with empty slice should not error: %v", err)
	}

	// Receive from non-existent vertex
	msgs, err := bus.Receive(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Receive from nonexistent vertex should not error: %v", err)
	}
	if msgs != nil {
		t.Errorf("Expected nil messages from nonexistent vertex, got %d", len(msgs))
	}

	// Clear non-existent vertex
	if err := bus.Clear(ctx, "nonexistent"); err != nil {
		t.Errorf("Clear nonexistent vertex should not error: %v", err)
	}

	// Stats with no messages
	stats, err := bus.Stats(ctx)
	if err != nil {
		t.Errorf("Stats with no messages should not error: %v", err)
	}
	if stats.TotalMessages != 0 || stats.VerticesWithMessages != 0 || stats.LargestMailbox != 0 {
		t.Errorf("Expected zero stats, got %+v", stats)
	}
}

// TestRedisMessageBus_ClosedOperations tests operations after Close
func TestRedisMessageBus_ClosedOperations(t *testing.T) {
	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer container.Terminate(ctx)

	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	bus, err := predis.NewMessageBus[string](addr, "", 0, nil)
	if err != nil {
		t.Fatalf("Failed to create message bus: %v", err)
	}

	// Close the bus
	if err := bus.Close(); err != nil {
		t.Fatalf("Failed to close bus: %v", err)
	}

	// Verify operations fail after close
	if err := bus.Send(ctx, []pregel.Message[string]{{To: "node", Data: "test"}}); err == nil {
		t.Error("Expected error when sending after close")
	}

	if _, err := bus.Receive(ctx, "node"); err == nil {
		t.Error("Expected error when receiving after close")
	}

	if err := bus.Clear(ctx, "node"); err == nil {
		t.Error("Expected error when clearing after close")
	}

	// Close should be idempotent
	if err := bus.Close(); err != nil {
		t.Errorf("Second close should not error: %v", err)
	}
}
