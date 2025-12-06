# Structured Output Example

This example demonstrates how to use AgentMesh's structured output capabilities to constrain model responses to specific JSON schemas.

## Overview

AgentMesh provides comprehensive structured output support through the `pkg/schema` package, allowing you to:

- 🎯 **Define schemas from Go structs** with automatic JSON Schema generation
- ✅ **Validate responses** against your schema with detailed error messages  
- 📊 **Extract typed data** directly from model responses
- 🔧 **Integrate seamlessly** with ModelNode and ReActAgent

## Features Demonstrated

### 1. Direct Model Usage with OutputSchema

Shows how to create an `OutputSchema` from a Go struct and use it directly with model requests:

```go
type MovieReview struct {
    Title      string   `json:"title" jsonschema:"required,description=Movie title"`
    Rating     int      `json:"rating" jsonschema:"required,minimum=1,maximum=5"`
    Summary    string   `json:"summary" jsonschema:"required"`
    Pros       []string `json:"pros"`
    Cons       []string `json:"cons"`
    Recommend  bool     `json:"recommend" jsonschema:"required"`
    Confidence float64  `json:"confidence" jsonschema:"required,minimum=0,maximum=1"`
}

// Create schema with metadata
outputSchema, _ := schema.NewOutputSchema("movie_review", MovieReview{},
    schema.WithStrict(true),
    schema.WithDescription("Structured movie review with rating and analysis"),
)

// Use in model request
req := &model.Request{
    Messages:     messages,
    OutputSchema: &outputSchema,
}
```

### 2. LastStructured Helper

Demonstrates the convenient `model.LastStructured[T]()` helper that automatically:
- Generates and waits for the model response
- Extracts the structured output
- Validates against the schema
- Unmarshals to your Go type

```go
analysis, err := model.LastStructured[ProductAnalysis](mdl.Generate(ctx, req))
// analysis is now a fully-typed ProductAnalysis struct
```

### 3. ReActAgent with Structured Output

Shows how to use structured output with ReActAgent for structured reasoning and responses:

```go
type AgentResponse struct {
    Reasoning  string  `json:"reasoning" jsonschema:"required"`
    Answer     string  `json:"answer" jsonschema:"required"`
    Confidence float64 `json:"confidence" jsonschema:"required,minimum=0,maximum=1"`
}

outputSchema, _ := schema.NewOutputSchema("agent_response", AgentResponse{},
    schema.WithStrict(true),
)

// Create agent with structured output
reactAgent, _ := agent.NewReAct(mdl,
    agent.WithReActOutputSchema(&outputSchema),
)

// Get structured response
response, err := schema.LastStructured[AgentResponse](
    reactAgent.Run(ctx, []message.Message{...}),
)
```

## Running the Example

### Prerequisites

- Go 1.22 or later
- OpenAI API key (automatically detected if available in environment)

### Run

```bash
cd examples/structured_output
go run main.go
```

## Example Output

```
🎬 Structured Output Example - AgentMesh
=========================================

📋 Example 1: Direct Model Usage
----------------------------------
Schema Name: movie_review
Strict Mode: true
Description: Structured movie review with rating and analysis

Generating structured movie review...

📊 Structured Review:
  Title: Inception
  Rating: 5/5 ⭐
  Summary: A mind-bending masterpiece that explores dreams within dreams
  Pros: [Innovative concept Stunning visuals Excellent performances]
  Cons: [Complex plot may confuse some viewers]
  Recommend: true
  Confidence: 0.95

🔍 Example 2: LastStructured Helper
------------------------------------
Using model.LastStructured helper...

📊 Product Analysis (via LastStructured):
  Product: iPhone 15 Pro
  Category: electronics
  Price: $999.00
  Features: [Titanium design A17 Pro chip Advanced camera system]
  Rating: 4.5/5 ⭐
  In Stock: true

🤖 Example 3: ReActAgent with Structured Output
------------------------------------------------
Creating ReActAgent with schema: agent_response

Asking agent: What is the capital of France and why is it significant?

📊 Agent Response:
  Reasoning: Paris has been the capital since the 6th century and is France's political, economic, and cultural center
  Answer: The capital of France is Paris. It is significant as a global center of art, fashion, gastronomy, and culture.
  Confidence: 0.98

✅ All examples completed!
```

## Key Concepts

### Schema Definition

Use struct tags to define your schema:

- `json:"field_name"` - JSON field name
- `jsonschema:"required"` - Mark field as required
- `jsonschema:"description=..."` - Add field description
- `jsonschema:"minimum=X,maximum=Y"` - Numeric constraints
- `jsonschema:"enum=val1|val2"` - Enumerated values

### Schema Options

Configure your schema with functional options:

```go
schema.NewOutputSchema("name", MyStruct{},
    schema.WithStrict(true),                      // Enable strict mode
    schema.WithDescription("Schema description"), // Add description
    schema.WithAllowAdditionalProperties(false),  // Disallow extra fields
)
```

### OpenAI Integration

The example uses OpenAI's GPT-4 with structured output support:

- `outputSchema.Name` - Used as the schema identifier
- `outputSchema.Strict` - Enables OpenAI's strict mode
- `outputSchema.Schema` - The JSON Schema definition

## Benefits

✅ **Type Safety**: Define schemas as Go structs for compile-time validation  
✅ **Validation**: Automatic schema validation with detailed error messages  
✅ **Metadata**: Include descriptions, constraints, and more in your schemas  
✅ **Provider Support**: Works with OpenAI, Anthropic, Google Gemini  
✅ **Easy Integration**: First-class support in ModelNode and ReActAgent  

## Further Reading

- [SCHEMA.md](../../SCHEMA.md) - Complete structured output documentation
- [pkg/schema](../../pkg/schema) - Schema package implementation
- [OpenAI Structured Outputs](https://platform.openai.com/docs/guides/structured-outputs) - Provider-specific details

## Notes

- Structured output requires model support (check `model.Capabilities().StructuredOutput`)
- OpenAI GPT-4 and later models support structured outputs natively
- The `SetModelResponseTool` provides a "tool trick" fallback for models without native support
- Use `WithStrict(true)` for OpenAI's strict mode (enforces exact schema adherence)
