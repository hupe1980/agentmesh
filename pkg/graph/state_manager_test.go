package graph

import (
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func TestStateCloneDeepCopy(t *testing.T) {
	state := NewStateManager(5)
	if err := state.Set("status", "pending"); err != nil {
		t.Fatalf("set status failed: %v", err)
	}

	state.AddMessages(wrapTestMessages([]message.Message{message.NewHumanMessageFromText("hello")}))

	clonedAny := state.Clone()
	cloned, ok := clonedAny.(*State)
	if !ok {
		t.Fatalf("expected *State clone, got %T", clonedAny)
	}

	if err := state.Set("status", "done"); err != nil {
		t.Fatalf("set status failed: %v", err)
	}
	state.AddMessages(wrapTestMessages([]message.Message{message.NewAIMessageFromText("world")}))

	if got := cloned.Get("status"); got != "pending" {
		t.Fatalf("clone should retain original status, got %v", got)
	}

	clonedMessages := cloned.MessageEventsSnapshot()
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

func TestStateSetMaxMessages(t *testing.T) {
	state := NewStateManager(0).(*State) // Type assert to access SetMaxMessages
	if err := state.Set("flag", true); err != nil {
		t.Fatalf("set flag failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		msg := message.NewHumanMessageFromText(fmt.Sprintf("msg-%d", i))
		state.AddMessages(wrapTestMessages([]message.Message{msg}))
	}

	state.SetMaxMessages(3)

	messages := state.MessageEventsSnapshot()
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

func wrapTestMessages(msgs []message.Message) []MessageEvent {
	events := make([]MessageEvent, len(msgs))
	for i, msg := range msgs {
		events[i] = *NewMessageEvent(msg, "", "test")
	}
	return events
}
