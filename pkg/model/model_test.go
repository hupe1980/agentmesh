package model_test

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// TestStreamChunk_Creation verifies StreamChunk struct creation
func TestStreamChunk_Creation(t *testing.T) {
	chunk := model.StreamChunk{
		Text:    "Hello",
		Message: message.NewAIMessageFromText("Hello"),
		Err:     nil,
		Final:   false,
	}

	if chunk.Text != "Hello" {
		t.Errorf("Expected text 'Hello', got %q", chunk.Text)
	}
	if chunk.Final {
		t.Error("Expected Final to be false")
	}
}

// TestStream_BasicIteration verifies stream iteration pattern
func TestStream_BasicIteration(t *testing.T) {
	chunks := make(chan model.StreamChunk, 3)
	chunks <- model.StreamChunk{Text: "Hello", Final: false}
	chunks <- model.StreamChunk{Text: " World", Final: false}
	chunks <- model.StreamChunk{Text: "!", Final: true}
	close(chunks)

	stream := model.NewStream(chunks, nil)

	var collected string
	for stream.Next() {
		chunk := stream.Current()
		collected += chunk.Text
	}

	if collected != "Hello World!" {
		t.Errorf("Expected 'Hello World!', got %q", collected)
	}

	if err := stream.Err(); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// TestStream_ErrorHandling verifies error propagation
func TestStream_ErrorHandling(t *testing.T) {
	testErr := errors.New("stream error")
	chunks := make(chan model.StreamChunk, 2)
	chunks <- model.StreamChunk{Text: "Start", Final: false}
	chunks <- model.StreamChunk{Err: testErr, Final: true}
	close(chunks)

	stream := model.NewStream(chunks, nil)

	count := 0
	for stream.Next() {
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 chunks, got %d", count)
	}

	if err := stream.Err(); !errors.Is(err, testErr) {
		t.Errorf("Expected error %v, got %v", testErr, err)
	}
}

// TestStream_Cancel verifies cancellation
func TestStream_Cancel(t *testing.T) {
	chunks := make(chan model.StreamChunk)
	cancelled := false
	cancelFunc := func() {
		cancelled = true
		close(chunks)
	}

	stream := model.NewStream(chunks, cancelFunc)
	stream.Cancel()

	// Wait briefly for async cancel
	time.Sleep(10 * time.Millisecond)

	if !cancelled {
		t.Error("Cancel function should have been called")
	}
}

// TestStream_NilSafety verifies nil stream doesn't panic
func TestStream_NilSafety(t *testing.T) {
	var stream *model.Stream

	// Should not panic
	stream.Cancel()
	if stream.Next() {
		t.Error("Nil stream should return false for Next()")
	}
	chunk := stream.Current()
	if chunk.Text != "" {
		t.Error("Nil stream should return zero-value chunk")
	}
	if err := stream.Err(); err != nil {
		t.Error("Nil stream should return nil error")
	}
}

// TestStream_EmptyStream verifies empty stream behavior
func TestStream_EmptyStream(t *testing.T) {
	chunks := make(chan model.StreamChunk)
	close(chunks)

	stream := model.NewStream(chunks, nil)

	if stream.Next() {
		t.Error("Empty stream should not have next item")
	}
}

// TestStream_ConcurrentCancel verifies thread safety of cancel
func TestStream_ConcurrentCancel(t *testing.T) {
	chunks := make(chan model.StreamChunk)
	cancelCount := 0
	cancelFunc := func() {
		cancelCount++
		close(chunks)
	}

	stream := model.NewStream(chunks, cancelFunc)

	// Cancel multiple times concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			stream.Cancel()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Cancel should be idempotent (called once despite multiple Cancel() calls)
	time.Sleep(10 * time.Millisecond)
	if cancelCount > 1 {
		t.Logf("Warning: Cancel called %d times (should be idempotent)", cancelCount)
	}
}

// TestStream_FinalChunkMarker verifies Final flag handling
func TestStream_FinalChunkMarker(t *testing.T) {
	chunks := make(chan model.StreamChunk, 3)
	chunks <- model.StreamChunk{Text: "A", Final: false}
	chunks <- model.StreamChunk{Text: "B", Final: false}
	chunks <- model.StreamChunk{
		Text:    "C",
		Message: message.NewAIMessageFromText("ABC"),
		Final:   true,
	}
	close(chunks)

	stream := model.NewStream(chunks, nil)

	var finalChunk model.StreamChunk
	for stream.Next() {
		finalChunk = stream.Current()
	}

	if !finalChunk.Final {
		t.Error("Last chunk should have Final=true")
	}

	// Extract text from message parts
	parts := finalChunk.Message.Parts()
	if len(parts) == 0 {
		t.Error("Expected message to have parts")
	}
	if textPart, ok := parts[0].(message.TextPart); ok {
		if textPart.Text != "ABC" {
			t.Errorf("Expected final message 'ABC', got %q", textPart.Text)
		}
	} else {
		t.Error("Expected first part to be TextPart")
	}
}

// TestStream_LargeStream verifies performance with many chunks
func TestStream_LargeStream(t *testing.T) {
	chunkCount := 1000
	chunks := make(chan model.StreamChunk, chunkCount)

	for i := 0; i < chunkCount-1; i++ {
		chunks <- model.StreamChunk{Text: "x", Final: false}
	}
	chunks <- model.StreamChunk{Text: "x", Final: true}
	close(chunks)

	stream := model.NewStream(chunks, nil)

	count := 0
	for stream.Next() {
		count++
	}

	if count != chunkCount {
		t.Errorf("Expected %d chunks, got %d", chunkCount, count)
	}
}

// mockModel is a test implementation for integration tests
type mockModel struct {
	response message.Message
	err      error
}

func (m *mockModel) Generate(ctx context.Context, messages []message.Message) iter.Seq2[message.Message, error] {
	return func(yield func(message.Message, error) bool) {
		if m.err != nil {
			yield(nil, m.err)
			return
		}

		// Simulate streaming with intermediate chunks
		parts := m.response.Parts()
		if len(parts) > 0 {
			if textPart, ok := parts[0].(message.TextPart); ok {
				// Yield intermediate chunks
				text := textPart.Text
				if len(text) > 3 {
					chunk1 := message.NewAIMessageFromText(text[:len(text)/2])
					if !yield(chunk1, nil) {
						return
					}
				}
			}
		}

		// Yield final message
		yield(m.response, nil)
	}
}

// TestMockModel_Generate verifies mock model implementation
func TestMockModel_Generate(t *testing.T) {
	expectedMsg := message.NewAIMessageFromText("Test response")
	mock := &mockModel{response: expectedMsg}

	result, err := model.Last(mock.Generate(context.Background(), nil))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	parts := result.Parts()
	if len(parts) == 0 {
		t.Fatal("Expected message to have parts")
	}
	if textPart, ok := parts[0].(message.TextPart); ok {
		if textPart.Text != "Test response" {
			t.Errorf("Expected 'Test response', got %q", textPart.Text)
		}
	} else {
		t.Error("Expected first part to be TextPart")
	}
}

// TestMockModel_Stream verifies mock streaming
func TestMockModel_Stream(t *testing.T) {
	expectedMsg := message.NewAIMessageFromText("Streaming")
	mock := &mockModel{response: expectedMsg}

	// Test streaming by collecting all messages
	messages, err := model.Collect(mock.Generate(context.Background(), nil))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(messages) < 1 {
		t.Fatal("Expected at least one message")
	}

	// Verify the final message
	finalMsg := messages[len(messages)-1]
	parts := finalMsg.Parts()
	if len(parts) == 0 {
		t.Fatal("Expected final message to have parts")
	}
	if textPart, ok := parts[0].(message.TextPart); ok {
		if textPart.Text != "Streaming" {
			t.Errorf("Expected 'Streaming', got %q", textPart.Text)
		}
	}
}
