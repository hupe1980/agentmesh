/*
Package model provides interfaces and adapters for integrating Large Language Models
(LLMs) into agent workflows.

# Overview

The model package defines a common interface for LLM providers, enabling you to:
  - Use multiple LLM providers (OpenAI, Anthropic, LangChainGo adapters)
  - Support tool/function calling
  - Stream responses in real-time
  - Handle structured outputs

# Supported Providers

  - OpenAI: GPT-4, GPT-3.5-turbo via OpenAI API
  - Anthropic: Claude models via AWS Bedrock
  - LangChainGo: Adapter for LangChain-compatible models

# Quick Start

Using OpenAI:

	import (
		"github.com/hupe1980/agentmesh/pkg/model"
		"github.com/hupe1980/agentmesh/pkg/model/openai"
	)

	llm := openai.NewModel(
		openai.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
		openai.WithModelName("gpt-4"),
	)

	response, _ := llm.Generate(ctx, []message.Message{
		message.NewSystemMessageFromText("You are a helpful assistant"),
		message.NewHumanMessageFromText("Hello!"),
	})

Using Anthropic (AWS Bedrock):

	import "github.com/hupe1980/agentmesh/pkg/model/anthropic"

	llm := anthropic.NewModel(
		anthropic.WithModelID("anthropic.claude-v2"),
		anthropic.WithRegion("us-east-1"),
	)

# Tool Calling

Models that implement ToolAware can invoke tools:

	import "github.com/hupe1980/agentmesh/pkg/tool"

	weatherTool, _ := tool.NewFuncTool("get_weather", "Get weather", weatherFunc)

	// Check if model supports tools
	if toolAware, ok := llm.(model.ToolAware); ok {
		response, _ := toolAware.GenerateWithTools(ctx, messages, []tool.Interface{weatherTool})
	}

# Streaming

Stream responses token-by-token:

	seq := llm.Generate(ctx, &model.Request{
		Messages: messages,
		Stream:   true,
	})

	for res, err := range seq {
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		if res.Partial {
			fmt.Print(res.Message.String())
		} else {
			fmt.Println(res.Message.String())
		}
	}

# Model Interface

All models implement the core Interface:

	type Interface interface {
		Generate(ctx context.Context, messages []message.Message, opts ...Option) (message.Message, error)
		Stream(ctx context.Context, messages []message.Message, opts ...Option) <-chan StreamEvent
	}

# Tool-Aware Models

Models with function calling implement ToolAware:

	type ToolAware interface {
		Interface
		GenerateWithTools(ctx context.Context, messages []message.Message, tools []tool.Interface, opts ...Option) (message.Message, error)
	}

# Configuration Options

Customize model behavior with options:

	llm.Generate(ctx, messages,
		model.WithTemperature(0.7),
		model.WithMaxTokens(500),
		model.WithTopP(0.9),
	)

# Custom Model Adapters

Implement the Interface to add new providers:

	type CustomModel struct {
		client *CustomClient
	}

	func (m *CustomModel) Generate(ctx context.Context, messages []message.Message, opts ...Option) (message.Message, error) {
		// Convert messages to provider format
		// Call provider API
		// Convert response to message.Message
		return response, nil
	}

	func (m *CustomModel) Stream(ctx context.Context, messages []message.Message, opts ...Option) <-chan StreamEvent {
		ch := make(chan StreamEvent)
		go func() {
			defer close(ch)
			// Stream from provider
		}()
		return ch
	}
*/
package model
