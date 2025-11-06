package graph

import (
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func TestGraphStateCloneDeepCopy(t *testing.T) {
	state := NewGraphState(5)
	if err := state.Set("status", "pending"); err != nil {
		t.Fatalf("set status failed: %v", err)
	}

	msg1 := message.NewHumanMessageFromText("hello")
	state.AddMessages([]message.Message{msg1})

	clonedAny := state.Clone()
	cloned, ok := clonedAny.(*GraphState)
	if !ok {
		t.Fatalf("expected *GraphState clone, got %T", clonedAny)
	}

	if err := state.Set("status", "done"); err != nil {
		t.Fatalf("set status failed: %v", err)
	}
	msg2 := message.NewAIMessageFromText("world")
	state.AddMessages([]message.Message{msg2})

	if got := cloned.Get("status"); got != "pending" {
		t.Fatalf("clone should retain original status, got %v", got)
	}

	clonedMessages := cloned.MessagesSnapshot()
	if len(clonedMessages) != 1 {
		t.Fatalf("expected 1 message in clone, got %d", len(clonedMessages))
	}

	ch, ok := cloned.GetChannel("messages")
	if !ok {
		t.Fatal("clone missing messages channel")
	}
	topic, ok := ch.(*channel.TopicChannel)
	if !ok {
		t.Fatalf("expected TopicChannel, got %T", ch)
	}
	if topic.MaxValues() != 5 {
		t.Fatalf("expected message limit 5, got %d", topic.MaxValues())
	}
}

func TestGraphStateSetMaxMessages(t *testing.T) {
	state := NewGraphState(0)
	if err := state.Set("flag", true); err != nil {
		t.Fatalf("set flag failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		msg := message.NewHumanMessageFromText(fmt.Sprintf("msg-%d", i))
		state.AddMessages([]message.Message{msg})
	}

	state.SetMaxMessages(3)

	messages := state.MessagesSnapshot()
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages retained, got %d", len(messages))
	}

	for idx, m := range messages {
		expected := fmt.Sprintf("msg-%d", idx+2)
		parts := m.Parts()
		if len(parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(parts))
		}
		textPart, ok := parts[0].(message.TextPart)
		if !ok {
			t.Fatalf("expected TextPart, got %T", parts[0])
		}
		if textPart.Text != expected {
			t.Fatalf("expected %s, got %s", expected, textPart.Text)
		}
	}

	// Ensure non-message channels remain intact
	if val := state.Get("flag"); val != true {
		t.Fatalf("expected flag to remain true, got %v", val)
	}
}
