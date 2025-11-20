/*
Package tool provides abstractions for defining and executing tools that agents
can use to perform actions and retrieve information.

# Overview

Tools are functions or objects that agents can invoke to interact with external
systems, perform computations, or retrieve data. The tool package provides:
  - Automatic JSON schema generation for parameters
  - Type-safe function wrapping
  - Structured tool interfaces

# Quick Start

Create a tool from a function:

	import (
		"context"
		"github.com/hupe1980/agentmesh/pkg/tool"
	)

	weatherTool, err := tool.NewFuncTool(
		"get_weather",
		"Get current weather for a given location",
		func(ctx context.Context, args struct {
			Location string `json:"location" description:"City name"`
			Units    string `json:"units,omitempty" description:"Temperature units (celsius/fahrenheit)"`
		}) (any, error) {
			// Implementation...
			return map[string]any{
				"temperature": 72,
				"conditions":  "Sunny",
				"location":    args.Location,
			}, nil
		},
	)

# Tool Interface

Implement the Interface to create custom tools:

	type Interface interface {
		Name() string
		Description() string
		JSONSchema() (map[string]any, error)
		Run(ctx context.Context, input string) (any, error)
	}

	type CustomTool struct{}

	func (t *CustomTool) Name() string {
		return "custom"
	}

	func (t *CustomTool) Description() string {
		return "A custom tool"
	}

	func (t *CustomTool) JSONSchema() (map[string]any, error) {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]string{"type": "string"},
			},
		}, nil
	}

	func (t *CustomTool) Run(ctx context.Context, input string) (any, error) {
		// Parse input JSON and execute
		return result, nil
	}

# Function Tools

NewFuncTool automatically generates JSON schemas from struct tags:

	type SearchArgs struct {
		Query    string   `json:"query" description:"Search query"`
		MaxResults int    `json:"max_results,omitempty" description:"Maximum number of results"`
		Filters  []string `json:"filters,omitempty" description:"Filter categories"`
	}

	searchTool, _ := tool.NewFuncTool(
		"search",
		"Search the knowledge base",
		func(ctx context.Context, args SearchArgs) ([]string, error) {
			// Implementation...
			return results, nil
		},
	)

# Supported Types

Function tools support various parameter types:
  - Primitives: string, int, float64, bool
  - Structs: Nested objects with JSON tags
  - Slices: Arrays of any supported type
  - Maps: map[string]any for flexible schemas
  - Pointers: Optional fields with omitempty

# Error Handling

Tool errors are returned to the agent:

	func (ctx context.Context, args Args) (any, error) {
		if args.Query == "" {
			return nil, fmt.Errorf("query cannot be empty")
		}
		// Error returned to agent as tool result
	}

# Context Handling

Tools receive context for cancellation and timeouts:

	func (ctx context.Context, args Args) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-performLongOperation():
			return result, nil
		}
	}

# Best Practices

  - Keep tools focused (single responsibility)
  - Use descriptive names and descriptions
  - Provide detailed JSON schema descriptions
  - Handle errors gracefully
  - Support context cancellation
  - Return structured data when possible
*/
package tool
