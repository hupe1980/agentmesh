// Package schema provides utilities for creating and working with JSON schemas
// for structured output in AgentMesh.
//
// # Overview
//
// The schema package enables type-safe structured output from language models by:
//   - Generating JSON schemas from Go structs with jsonschema tags
//   - Creating schemas from maps for flexibility
//   - Validating values against schemas
//   - Extracting and parsing structured outputs from model responses
//
// # Basic Usage
//
// Define a struct with jsonschema tags:
//
//	type Person struct {
//	    Name  string `json:"name" jsonschema:"required,description=Person's full name"`
//	    Age   int    `json:"age" jsonschema:"required,minimum=0,maximum=150"`
//	    Email string `json:"email" jsonschema:"format=email"`
//	}
//
// Create an OutputSchema:
//
//	outputSchema, err := schema.NewOutputSchema("person", Person{},
//	    schema.WithStrict(true),
//	    schema.WithDescription("Extracts person information"),
//	)
//
// Use with model requests:
//
//	req := &model.Request{
//	    Messages:     messages,
//	    OutputSchema: &outputSchema,
//	}
//
// Extract structured output:
//
//	person, err := model.LastStructured[Person](mdl.Generate(ctx, req))
//
// # Supported Tags
//
// The package supports standard jsonschema struct tags:
//   - required: Mark field as required
//   - description: Field description
//   - minimum, maximum: Numeric constraints
//   - minLength, maxLength: String length constraints
//   - enum: Allowed values (comma-separated)
//   - format: Format validation (email, date-time, uri, etc.)
//   - default: Default value
//
// # Helper Functions
//
// LastStructured extracts and parses the last message from an agent or graph:
//
//	result, err := schema.LastStructured[AnalysisResult](compiled.Run(ctx, input))
//
// For model-level responses, use model.LastStructured:
//
//	result, err := model.LastStructured[MovieReview](mdl.Generate(ctx, req))
//
// # Validation
//
// Validate values against schemas:
//
//	value := map[string]any{"name": "John", "age": 30}
//	err := schema.Validate(outputSchema.Schema, value)
package schema
