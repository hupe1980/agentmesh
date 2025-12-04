// Package amazonbedrock provides a model adapter for Amazon Bedrock foundation models.
//
// Amazon Bedrock is a fully managed service that offers a choice of high-performing
// foundation models from leading AI companies through a single API. This adapter
// uses the Converse API which provides a unified interface across different model
// providers (Anthropic Claude, Meta Llama, Amazon Titan, Mistral, etc.).
//
// # Quick Start
//
// The simplest way to create a Bedrock model:
//
//	import (
//	    "context"
//	    "github.com/aws/aws-sdk-go-v2/config"
//	    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
//	    "github.com/hupe1980/agentmesh/pkg/model/amazonbedrock"
//	)
//
//	cfg, _ := config.LoadDefaultConfig(context.Background())
//	client := bedrockruntime.NewFromConfig(cfg)
//	model := amazonbedrock.NewModel(client)
//
// # Model Selection
//
// Bedrock supports multiple foundation models. You can use either direct model IDs
// or cross-region inference profile IDs.
//
// Direct model IDs (may require on-demand access or use case forms):
//
//	model := amazonbedrock.NewModel(client,
//	    amazonbedrock.WithModelID("anthropic.claude-3-haiku-20240307-v1:0"),
//	)
//
// Cross-region inference profiles (recommended for production):
//
//	model := amazonbedrock.NewModel(client,
//	    amazonbedrock.WithModelID("eu.amazon.nova-pro-v1:0"),  // EU region
//	)
//
// Common inference profile IDs (use region prefix: us., eu., etc.):
//   - eu.amazon.nova-pro-v1:0 / us.amazon.nova-pro-v1:0 (Nova Pro with tools)
//   - eu.amazon.nova-lite-v1:0 / us.amazon.nova-lite-v1:0 (Nova Lite with tools)
//   - eu.anthropic.claude-3-haiku-20240307-v1:0 (Claude 3 Haiku)
//   - eu.anthropic.claude-3-5-sonnet-20240620-v1:0 (Claude 3.5 Sonnet)
//   - eu.meta.llama3-2-3b-instruct-v1:0 (Llama 3.2)
//
// To list available inference profiles:
//
//	aws bedrock list-inference-profiles --query "inferenceProfileSummaries[].inferenceProfileId"
//
// # Tool/Function Calling
//
// The adapter supports tool calling for models that support it:
//
//	req := &model.Request{
//	    Messages: messages,
//	    Tools:    myTools,
//	}
//	resp, err := model.Last(mdl.Generate(ctx, req))
//
// # Streaming
//
// Streaming is supported via the ConverseStream API:
//
//	req := &model.Request{
//	    Messages: messages,
//	    Stream:   true,
//	}
//	for resp, err := range mdl.Generate(ctx, req) {
//	    // handle response
//	}
package amazonbedrock
