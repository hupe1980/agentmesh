package main

import (
	"context"
	"flag"
	"log"

	a2atypes "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	cardURL = flag.String("card-url", "http://127.0.0.1:9001", "Base URL of the AgentCard server")
)

func main() {
	flag.Parse()

	ctx := context.Background()

	// Resolve the AgentCard
	log.Printf("Resolving AgentCard from %s...", *cardURL)
	card, err := agentcard.DefaultResolver.Resolve(ctx, *cardURL)
	if err != nil {
		log.Fatalf("Failed to resolve AgentCard: %v", err)
	}

	log.Printf("✓ Found agent: %s", card.Name)
	log.Printf("  Description: %s", card.Description)
	log.Printf("  Skills: %d available", len(card.Skills))

	// List available skills
	for i, skill := range card.Skills {
		log.Printf("    %d. %s: %s", i+1, skill.Name, skill.Description)
	}

	// Create an A2A client (using insecure connection for local development)
	withInsecureGRPC := a2aclient.WithGRPCTransport(grpc.WithTransportCredentials(insecure.NewCredentials()))
	client, err := a2aclient.NewFromCard(ctx, card, withInsecureGRPC)
	if err != nil {
		log.Fatalf("Failed to create A2A client: %v", err)
	}

	// Send a test message
	log.Printf("\nSending test message...")
	msg := a2atypes.NewMessage("user", a2atypes.TextPart{
		Text: "Generate a greeting for Alice",
	})

	resp, err := client.SendMessage(ctx, &a2atypes.MessageSendParams{
		Message: msg,
	})
	if err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}

	// Extract and display the response
	log.Printf("✓ Agent Response:")
	switch r := resp.(type) {
	case *a2atypes.Task:
		// Debug: Show what's in the task
		log.Printf("  [DEBUG] Task ID: %s", r.ID)
		log.Printf("  [DEBUG] Artifacts: %d", len(r.Artifacts))
		log.Printf("  [DEBUG] Status.Message: %v", r.Status.Message != nil)
		log.Printf("  [DEBUG] History: %d", len(r.History))
		log.Printf("  [DEBUG] Status.State: %s", r.Status.State)

		// Task response - extract message from artifacts, status, or history (priority order)
		var responseMsg *a2atypes.Message
		if len(r.Artifacts) > 0 {
			// Priority 1: Use last artifact's parts
			lastArtifact := r.Artifacts[len(r.Artifacts)-1]
			log.Printf("  [DEBUG] Using artifact with %d parts", len(lastArtifact.Parts))
			responseMsg = &a2atypes.Message{
				Role:  a2atypes.MessageRoleAgent,
				Parts: lastArtifact.Parts,
			}
		} else if r.Status.Message != nil {
			// Priority 2: Use status message
			log.Printf("  [DEBUG] Using status message with %d parts", len(r.Status.Message.Parts))
			responseMsg = r.Status.Message
		} else if len(r.History) > 0 {
			// Priority 3: Use last history message
			lastMsg := r.History[len(r.History)-1]
			log.Printf("  [DEBUG] Using history message with %d parts, role: %s", len(lastMsg.Parts), lastMsg.Role)
			responseMsg = lastMsg
		}

		if responseMsg != nil {
			foundText := false
			for i, part := range responseMsg.Parts {
				log.Printf("  [DEBUG] Part %d type: %T", i, part)
				if textPart, ok := part.(a2atypes.TextPart); ok {
					log.Printf("  %s", textPart.Text)
					foundText = true
				}
			}
			if !foundText {
				log.Printf("  [DEBUG] No text parts found in message")
			}
		} else {
			log.Printf("  (No message content in task)")
		}
	case *a2atypes.Message:
		// Direct message response
		for _, part := range r.Parts {
			switch p := part.(type) {
			case a2atypes.TextPart:
				log.Printf("  %s", p.Text)
			case a2atypes.DataPart:
				// DataPart contains structured data - format it nicely
				for key, value := range p.Data {
					log.Printf("  %s: %v", key, value)
				}
			case a2atypes.FilePart:
				log.Printf("  [File: %+v]", p.File)
			default:
				log.Printf("  [Unknown part type: %T]", part)
			}
		}
	default:
		log.Printf("  [DEBUG] Received unexpected response type: %T", resp)
	}

	// Test streaming
	log.Printf("\nTesting streaming message...")
	streamMsg := a2atypes.NewMessage("user", a2atypes.TextPart{
		Text: "What tools do you have available?",
	})

	stream := client.SendStreamingMessage(ctx, &a2atypes.MessageSendParams{
		Message: streamMsg,
	})

	log.Printf("✓ Streaming Response:")
	for event, err := range stream {
		if err != nil {
			log.Printf("  Streaming error: %v", err)
			break
		}

		// Extract message from different event types
		switch e := event.(type) {
		case *a2atypes.Task:
			// Task event - check artifacts, status, or history
			if len(e.Artifacts) > 0 {
				lastArtifact := e.Artifacts[len(e.Artifacts)-1]
				for _, part := range lastArtifact.Parts {
					if textPart, ok := part.(a2atypes.TextPart); ok {
						log.Printf("  [Artifact] %s", textPart.Text)
					}
				}
			} else if e.Status.Message != nil {
				for _, part := range e.Status.Message.Parts {
					if textPart, ok := part.(a2atypes.TextPart); ok {
						log.Printf("  [Status] %s", textPart.Text)
					}
				}
			} else if len(e.History) > 0 {
				lastMsg := e.History[len(e.History)-1]
				for _, part := range lastMsg.Parts {
					if textPart, ok := part.(a2atypes.TextPart); ok {
						log.Printf("  [History] %s", textPart.Text)
					}
				}
			}
		case *a2atypes.Message:
			// Direct message event
			for _, part := range e.Parts {
				if textPart, ok := part.(a2atypes.TextPart); ok {
					log.Printf("  %s", textPart.Text)
				}
			}
		case *a2atypes.TaskStatusUpdateEvent:
			// Status update with optional message
			if e.Status.Message != nil {
				for _, part := range e.Status.Message.Parts {
					if textPart, ok := part.(a2atypes.TextPart); ok {
						log.Printf("  [Update] %s", textPart.Text)
					}
				}
			}
		case *a2atypes.TaskArtifactUpdateEvent:
			// Artifact chunk update
			if e.Artifact != nil {
				for _, part := range e.Artifact.Parts {
					if textPart, ok := part.(a2atypes.TextPart); ok {
						log.Printf("  [Artifact Update] %s", textPart.Text)
					}
				}
			}
		}
	}

	log.Printf("\n✅ Client test completed successfully!")
}
