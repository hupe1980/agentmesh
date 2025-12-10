package testutil

import (
	"strings"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// MessageMatcher is a function that validates a message.
type MessageMatcher func(msg message.Message) bool

// IsHuman creates a matcher for human messages with the given content.
func IsHuman(content string) MessageMatcher {
	return func(msg message.Message) bool {
		if msg.Type() != message.TypeHuman {
			return false
		}
		return strings.Contains(msg.String(), content)
	}
}

// IsAI creates a matcher for AI messages. Optionally with a content matcher.
func IsAI(opts ...func(msg message.Message) bool) MessageMatcher {
	return func(msg message.Message) bool {
		if msg.Type() != message.TypeAI {
			return false
		}
		for _, opt := range opts {
			if !opt(msg) {
				return false
			}
		}
		return true
	}
}

// IsTool creates a matcher for tool result messages.
func IsTool(toolCallID string) MessageMatcher {
	return func(msg message.Message) bool {
		if msg.Type() != message.TypeTool {
			return false
		}
		tm, ok := msg.(*message.ToolMessage)
		if !ok {
			return false
		}
		return tm.ToolCallID == toolCallID
	}
}

// IsSystem creates a matcher for system messages.
func IsSystem() MessageMatcher {
	return func(msg message.Message) bool {
		return msg.Type() == message.TypeSystem
	}
}

// Contains creates a content matcher that checks for substring presence.
func Contains(text string) func(msg message.Message) bool {
	return func(msg message.Message) bool {
		return strings.Contains(msg.String(), text)
	}
}

// HasToolCall creates a matcher that checks for a tool call with the given name.
func HasToolCall(toolName string) func(msg message.Message) bool {
	return func(msg message.Message) bool {
		aiMsg, ok := msg.(*message.AIMessage)
		if !ok {
			return false
		}
		for _, tc := range aiMsg.ToolCalls {
			if tc.Name == toolName {
				return true
			}
		}
		return false
	}
}

// HasToolCallWithArgs creates a matcher that checks for a tool call with specific args.
func HasToolCallWithArgs(toolName string, argContains string) func(msg message.Message) bool {
	return func(msg message.Message) bool {
		aiMsg, ok := msg.(*message.AIMessage)
		if !ok {
			return false
		}
		for _, tc := range aiMsg.ToolCalls {
			if tc.Name == toolName && strings.Contains(tc.Arguments, argContains) {
				return true
			}
		}
		return false
	}
}

// AssertMessages validates a slice of messages against matchers in order.
func AssertMessages(t *testing.T, messages []message.Message, matchers ...MessageMatcher) {
	t.Helper()

	if len(messages) < len(matchers) {
		t.Errorf("expected at least %d messages, got %d", len(matchers), len(messages))
		return
	}

	for i, matcher := range matchers {
		if !matcher(messages[i]) {
			t.Errorf("message at index %d did not match: type=%s, content=%q",
				i, messages[i].Type(), truncate(messages[i].String(), 100))
		}
	}
}

// AssertMessagesContain checks that the messages contain at least one match for each matcher.
func AssertMessagesContain(t *testing.T, messages []message.Message, matchers ...MessageMatcher) {
	t.Helper()

	for i, matcher := range matchers {
		found := false
		for _, msg := range messages {
			if matcher(msg) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no message matched matcher at index %d", i)
		}
	}
}

// AssertEventually waits for a condition to become true.
func AssertEventually(t *testing.T, condition func() bool, timeout time.Duration, msgAndArgs ...any) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(msgAndArgs) > 0 {
		t.Error(msgAndArgs...)
	} else {
		t.Error("condition was not met within timeout")
	}
}

// AssertNever asserts that a condition never becomes true within the timeout.
func AssertNever(t *testing.T, condition func() bool, timeout time.Duration, msgAndArgs ...any) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			if len(msgAndArgs) > 0 {
				t.Error(msgAndArgs...)
			} else {
				t.Error("condition became true when it should not have")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// truncate shortens a string for display.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
