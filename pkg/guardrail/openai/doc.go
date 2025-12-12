// Package openai provides a guardrail implementation using OpenAI's Moderation API.
//
// The OpenAI Moderation API checks text for potentially harmful content across
// multiple categories including hate, harassment, self-harm, sexual content,
// and violence.
//
// # Basic Usage
//
//	import "github.com/openai/openai-go/v2"
//
//	// Create OpenAI client
//	oaiClient := openai.NewClient()
//
//	// Wrap it for the guardrail
//	client := openai.NewClientWrapper(oaiClient)
//
//	// Create guardrail
//	g := New(client)
//	result, err := g.Check(ctx, "text to check")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if !result.IsAllowed() {
//	    fmt.Println("Content flagged:", result.Message)
//	}
//
// # With Options
//
//	g := New(client,
//	    WithName("custom-moderation"),
//	    WithAction(guardrail.ActionRaise),
//	    WithThreshold(CategoryHate, 0.8),
//	    WithModel(openai.ModerationModelOmniModerationLatest),
//	)
//
// # Testing with Mock Client
//
//	type MockClient struct {
//	    ModerateFunc func(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error)
//	}
//
//	func (m *MockClient) Moderate(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
//	    return m.ModerateFunc(ctx, params)
//	}
//
//	client := &MockClient{
//	    ModerateFunc: func(ctx context.Context, params openai.ModerationNewParams) (*openai.ModerationNewResponse, error) {
//	        return &openai.ModerationNewResponse{Results: []openai.Moderation{{Flagged: false}}}, nil
//	    },
//	}
//	g := New(client)
package openai
