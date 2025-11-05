package message

import "testing"

func TestHumanMessage(t *testing.T) {
	msg := NewHumanMessageFromText("hello")
	if msg.Type() != TypeHuman {
		t.Fatalf("expected TypeHuman, got %s", msg.Type())
	}
	content := msg.Parts()
	if len(content) != 1 {
		t.Fatalf("expected 1 part, got %d", len(content))
	}
	text, ok := content[0].(TextPart)
	if !ok {
		t.Fatalf("expected TextPart, got %T", content[0])
	}
	if text.Text != "hello" {
		t.Fatalf("unexpected text content: %q", text.Text)
	}

	clone := msg.Clone().(*HumanMessage)
	content[0] = TextPart{Text: "mutated"}
	cloneText, _ := clone.Parts()[0].(TextPart)
	if cloneText.Text != "hello" {
		t.Fatalf("clone should preserve original text")
	}
}

func TestAIMessageCloneToolCalls(t *testing.T) {
	msg := NewAIMessageFromText("hi")
	msg.ToolCalls = []ToolCall{{
		ID:   "call-1",
		Name: "math",
		Type: "tool",
		Arguments: map[string]any{
			"x": 1,
		},
	}}

	clone := msg.Clone().(*AIMessage)
	msg.ToolCalls[0].Arguments["x"] = 42
	if clone.ToolCalls[0].Arguments["x"].(int) != 1 {
		t.Fatalf("expected clone to preserve original tool call arguments")
	}
}

func TestChunkMerge(t *testing.T) {
	first := NewBaseMessageChunk(TypeAI, "hello")
	second := NewBaseMessageChunk(TypeAI, " world")

	merged, err := first.Merge(second)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}
	parts := merged.Parts()
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	p0, _ := parts[0].(TextPart)
	p1, _ := parts[1].(TextPart)
	if p0.Text != "hello" || p1.Text != " world" {
		t.Fatalf("unexpected merged content: %q | %q", p0.Text, p1.Text)
	}

	other := NewBaseMessageChunk(TypeHuman, "alt")
	_, err = first.Merge(other)
	if err == nil {
		t.Fatalf("expected error when merging different types")
	}
}
