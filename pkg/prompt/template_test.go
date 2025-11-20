package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tmpl := New("Hello, {{.Name}}!")
	assert.NotNil(t, tmpl)
	assert.Equal(t, "Hello, {{.Name}}!", tmpl.String())
}

func TestRenderBasic(t *testing.T) {
	tmpl := New("Hello, {{.Name}}!")

	result, err := tmpl.Render(map[string]any{
		"Name": "Alice",
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello, Alice!", result)
}

func TestRenderMultipleVariables(t *testing.T) {
	tmpl := New("{{.Greeting}}, {{.Name}}! You are {{.Age}} years old.")

	result, err := tmpl.Render(map[string]any{
		"Greeting": "Hello",
		"Name":     "Bob",
		"Age":      30,
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello, Bob! You are 30 years old.", result)
}

func TestRenderMissingVariableUsesZeroValue(t *testing.T) {
	// With text/template and missingkey=zero, missing vars use zero values
	tmpl := New("Hello, {{.Name}}! You are {{.Age}}.")

	result, err := tmpl.Render(map[string]any{
		"Name": "Charlie",
		// Age is missing - will be 0
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello, Charlie! You are <no value>.", result)
}

func TestRenderComplexPrompt(t *testing.T) {
	tmpl := New(`You are a {{.Role}}.

Task: {{.Task}}

Instructions:
- Be {{.Tone}}

Please respond:`)

	result, err := tmpl.Render(map[string]any{
		"Role": "helpful assistant",
		"Task": "Summarize this article",
		"Tone": "professional",
	})

	require.NoError(t, err)
	assert.Contains(t, result, "You are a helpful assistant.")
	assert.Contains(t, result, "Task: Summarize this article")
}

func TestHelperDefault(t *testing.T) {
	tmpl := New("Role: {{default \"user\" .Role}}")

	// Missing key uses default
	result, err := tmpl.Render(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "Role: user", result)

	// Provided value overrides default
	result, err = tmpl.Render(map[string]any{"Role": "admin"})
	require.NoError(t, err)
	assert.Equal(t, "Role: admin", result)

	// Empty string uses default
	result, err = tmpl.Render(map[string]any{"Role": ""})
	require.NoError(t, err)
	assert.Equal(t, "Role: user", result)
}

func TestHelperUpper(t *testing.T) {
	tmpl := New("{{.Name | upper}}")

	result, err := tmpl.Render(map[string]any{"Name": "alice"})
	require.NoError(t, err)
	assert.Equal(t, "ALICE", result)
}

func TestHelperLower(t *testing.T) {
	tmpl := New("{{.Name | lower}}")

	result, err := tmpl.Render(map[string]any{"Name": "ALICE"})
	require.NoError(t, err)
	assert.Equal(t, "alice", result)
}

func TestHelperTitle(t *testing.T) {
	tmpl := New("{{.Name | title}}")

	var result string
	var err error
	result, err = tmpl.Render(map[string]any{"Name": "alice"})
	require.NoError(t, err)
	assert.Equal(t, "Alice", result)
}

func TestFastPathNoTemplate(t *testing.T) {
	tmpl := New("This is plain text")

	result, err := tmpl.Render(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "This is plain text", result)
}

func TestConditionals(t *testing.T) {
	tmpl := New("{{if .Premium}}Premium user{{else}}Standard user{{end}}")

	result, err := tmpl.Render(map[string]any{"Premium": true})
	require.NoError(t, err)
	assert.Equal(t, "Premium user", result)

	result, err = tmpl.Render(map[string]any{"Premium": false})
	require.NoError(t, err)
	assert.Equal(t, "Standard user", result)
}
