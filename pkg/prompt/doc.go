// Package prompt provides a simple template system for LLM prompts with variable substitution.
//
// # Overview
//
// The prompt package offers a lightweight alternative to text/template for common
// LLM prompt construction patterns. Templates support {{.Variable}} substitution
// with type-safe variable replacement.
//
// # Basic Usage
//
//	template := prompt.New("You are a {{.Role}}. Answer: {{.Question}}")
//	result, err := template.Render(map[string]any{
//	    "Role": "helpful assistant",
//	    "Question": "What is Go?",
//	})
//	// Result: "You are a helpful assistant. Answer: What is Go?"
//
// # Use Cases
//
//   - Reusable prompt templates across multiple agents
//   - Dynamic prompt construction with type-safe variables
//   - Reduced string concatenation boilerplate
//   - Consistent prompt formatting
//
// # Features
//
//   - Simple {{.Variable}} syntax
//   - Missing variable detection
//   - Automatic string conversion
//   - Safe for untrusted templates (no code execution)
package prompt
