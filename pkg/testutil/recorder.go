package testutil

import (
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// ConversationRecorder records model requests and responses for testing.
type ConversationRecorder struct {
	mu        sync.Mutex
	requests  []*model.Request
	responses []*model.Response
}

// NewConversationRecorder creates a new ConversationRecorder.
func NewConversationRecorder() *ConversationRecorder {
	return &ConversationRecorder{}
}

// RecordRequest records a model request.
func (r *ConversationRecorder) RecordRequest(req *model.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
}

// RecordResponse records a model response.
func (r *ConversationRecorder) RecordResponse(resp *model.Response) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responses = append(r.responses, resp)
}

// Requests returns all recorded requests.
func (r *ConversationRecorder) Requests() []*model.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*model.Request{}, r.requests...)
}

// Responses returns all recorded responses.
func (r *ConversationRecorder) Responses() []*model.Response {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*model.Response{}, r.responses...)
}

// RequestCount returns the number of requests made.
func (r *ConversationRecorder) RequestCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// ResponseCount returns the number of responses received.
func (r *ConversationRecorder) ResponseCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.responses)
}

// LastRequest returns the most recent request.
func (r *ConversationRecorder) LastRequest() *model.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		return nil
	}
	return r.requests[len(r.requests)-1]
}

// LastResponse returns the most recent response.
func (r *ConversationRecorder) LastResponse() *model.Response {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.responses) == 0 {
		return nil
	}
	return r.responses[len(r.responses)-1]
}

// AllMessages returns all messages from all requests, flattened.
func (r *ConversationRecorder) AllMessages() []message.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	var messages []message.Message
	for _, req := range r.requests {
		messages = append(messages, req.Messages...)
	}
	return messages
}

// Reset clears all recorded data.
func (r *ConversationRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
	r.responses = nil
}

// AssertRequestCount asserts that a specific number of requests were made.
func (r *ConversationRecorder) AssertRequestCount(t *testing.T, expected int) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) != expected {
		t.Errorf("expected %d requests, got %d", expected, len(r.requests))
	}
}

// AssertContains asserts that at least one message contains the given text.
func (r *ConversationRecorder) AssertContains(t *testing.T, text string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, req := range r.requests {
		for _, msg := range req.Messages {
			if containsText(msg.String(), text) {
				return
			}
		}
	}
	t.Errorf("no message contains text: %q", text)
}

// AssertToolCallMade asserts that a tool with the given name was called.
func (r *ConversationRecorder) AssertToolCallMade(t *testing.T, toolName string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, resp := range r.responses {
		if resp.Message != nil {
			if aiMsg, ok := resp.Message.(*message.AIMessage); ok {
				for _, tc := range aiMsg.ToolCalls {
					if tc.Name == toolName {
						return
					}
				}
			}
		}
	}
	t.Errorf("no tool call made for: %q", toolName)
}

// containsText checks if content contains the given text.
func containsText(content string, text string) bool {
	return content != "" && text != "" && contains(content, text)
}

// contains is a simple string contains check.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
