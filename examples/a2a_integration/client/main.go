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
	if msgGetter, ok := resp.(interface{ GetMessage() *a2atypes.Message }); ok {
		responseMsg := msgGetter.GetMessage()
		log.Printf("✓ Agent Response:")
		for _, part := range responseMsg.Parts {
			if textPart, ok := part.(a2atypes.TextPart); ok {
				log.Printf("  %s", textPart.Text)
			}
		}
	} else {
		log.Printf("Received response of type: %T", resp)
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

		if msgGetter, ok := event.(interface{ GetMessage() *a2atypes.Message }); ok {
			msg := msgGetter.GetMessage()
			for _, part := range msg.Parts {
				if textPart, ok := part.(a2atypes.TextPart); ok {
					log.Printf("  %s", textPart.Text)
				}
			}
		}
	}

	log.Printf("\n✅ Client test completed successfully!")
}
