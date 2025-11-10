package graph

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// MessageEvent wraps a message with execution metadata.
// Created by NodeAdapter after node execution.
// Implements message.Message interface by delegating to the wrapped message.
type MessageEvent struct {
	// Message content (from model/user code)
	Message message.Message

	// Execution metadata (runtime-agnostic)
	ID        string    // UUID event identifier (generated automatically)
	GraphID   string    // Graph run ID (hierarchical for subgraphs: "parent:child")
	Node      string    // Node that created this message
	Timestamp time.Time // Creation timestamp
}

// NewMessageEvent creates an event wrapping a message.
// Automatically generates a UUID for the event ID.
func NewMessageEvent(msg message.Message, graphID, node string) *MessageEvent {
	return &MessageEvent{
		Message:   msg,
		ID:        uuid.New().String(),
		GraphID:   graphID,
		Node:      node,
		Timestamp: time.Now(),
	}
}

// String returns a human-readable representation of the event.
func (e *MessageEvent) String() string {
	return fmt.Sprintf("[%s:%s] %s", e.GraphID, e.Node, e.Message.Type())
}

// --- message.Message interface implementation (delegation) ---

// Type returns the message type from the wrapped message.
func (e *MessageEvent) Type() message.Type {
	return e.Message.Type()
}

// Parts returns the message parts from the wrapped message.
func (e *MessageEvent) Parts() message.Parts {
	return e.Message.Parts()
}

// Clone creates a deep copy of the event and wrapped message.
func (e *MessageEvent) Clone() message.Message {
	return &MessageEvent{
		Message:   e.Message.Clone(),
		ID:        e.ID,
		GraphID:   e.GraphID,
		Node:      e.Node,
		Timestamp: e.Timestamp,
	}
}

// cloneMessageEvents creates a deep copy of a slice of MessageEvents.
func cloneMessageEvents(events []MessageEvent) []MessageEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]MessageEvent, 0, len(events))
	for _, evt := range events {
		cloned := evt.Clone()
		if clonedEvt, ok := cloned.(*MessageEvent); ok {
			out = append(out, *clonedEvt)
		}
	}
	return out
}

// ExtractMessages extracts the underlying messages from MessageEvents.
// Helper for accessing message content when MessageEvent metadata is not needed.
func ExtractMessages(events []MessageEvent) []message.Message {
	if len(events) == 0 {
		return nil
	}
	messages := make([]message.Message, len(events))
	for i, evt := range events {
		messages[i] = evt.Message
	}
	return messages
}
