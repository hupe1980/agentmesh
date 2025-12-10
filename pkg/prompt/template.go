package prompt

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"strings"
	"text/template"
)

var (
	// ErrMissingVariable is returned when a required template variable is not provided
	ErrMissingVariable = errors.New("prompt: missing template variable")

	// ErrInvalidTemplate is returned when the template syntax is malformed
	ErrInvalidTemplate = errors.New("prompt: invalid template syntax")
)

// Template represents a prompt template with placeholders for variable substitution.
// Uses Go's text/template for powerful, standard templating.
type Template struct {
	tmpl *template.Template
	raw  string
}

// New creates a new Template from the given template string.
// Uses Go's text/template syntax with additional helper functions.
//
// Helper functions:
//   - default: {{default "fallback" .Value}} - use fallback if Value is nil/empty
//   - upper: {{.Name | upper}} - convert to uppercase
//   - lower: {{.Name | lower}} - convert to lowercase
//   - title: {{.Name | title}} - capitalize first letter
//   - join: {{join ", " .Items}} - join slice with separator
//
// Example:
//
//	tmpl := prompt.New("Hello, {{.Name}}!")
//	tmpl := prompt.New("{{if .Premium}}Premium{{else}}Standard{{end}}")
//	tmpl := prompt.New("Role: {{default \"user\" .Role}}")
func New(templateStr string) *Template {
	// Fast path: no template markers
	if !strings.Contains(templateStr, "{{") {
		return &Template{raw: templateStr}
	}

	tmpl, err := template.New("prompt").Funcs(template.FuncMap{
		"default": func(defaultVal any, val any) any {
			if val == nil || val == "" {
				return defaultVal
			}
			return val
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": func(s string) string {
			if s == "" {
				return s
			}
			return strings.ToUpper(string(s[0])) + strings.ToLower(s[1:])
		},
		"join": func(sep string, items []any) string {
			strItems := make([]string, len(items))
			for i, item := range items {
				strItems[i] = fmt.Sprintf("%v", item)
			}
			return strings.Join(strItems, sep)
		},
	}).Option("missingkey=zero").Parse(templateStr)

	if err != nil {
		// Return template with error - will fail on Render
		return &Template{
			tmpl: nil,
			raw:  templateStr,
		}
	}

	return &Template{
		tmpl: tmpl,
		raw:  templateStr,
	}
}

// Render executes the template with the provided variables and returns the result.
// Returns an error if the template is malformed or required variables are missing.
//
// Example:
//
//	tmpl := prompt.New("Hello, {{.Name}}! You have {{.Count}} messages.")
//	result, err := tmpl.Render(map[string]any{
//	    "Name": "Alice",
//	    "Count": 5,
//	})
func (t *Template) Render(data map[string]any) (string, error) {
	// Fast path: no template
	if t.tmpl == nil {
		return t.raw, nil
	}

	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, data); err != nil {
		// Check if it's a missing key error
		if strings.Contains(err.Error(), "no entry for key") {
			return "", fmt.Errorf("%w: %w", ErrMissingVariable, err)
		}
		return "", fmt.Errorf("%w: %w", ErrInvalidTemplate, err)
	}

	return buf.String(), nil
}

// MustRender executes the template and panics if an error occurs.
// Only use in contexts where template errors are programmer errors (e.g., hardcoded templates).
//
// Example:
//
//	tmpl := prompt.New("Status: {{.Status}}")
//	result := tmpl.MustRender(map[string]any{"Status": "active"})
func (t *Template) MustRender(data map[string]any) string {
	result, err := t.Render(data)
	if err != nil {
		panic(fmt.Sprintf("template render failed: %v", err))
	}
	return result
}

// RenderOrDefault renders the template, using default values for missing variables.
// This is useful when optional variables may not always be provided.
//
// Example:
//
//	result := tmpl.RenderOrDefault(map[string]any{
//	    "Name": "Bob",
//	}, map[string]any{
//	    "Greeting": "Hello",
//	})
func (t *Template) RenderOrDefault(data map[string]any, defaults map[string]any) string {
	merged := make(map[string]any)

	// Copy defaults first
	maps.Copy(merged, defaults)

	// Override with provided data
	maps.Copy(merged, data)

	result, _ := t.Render(merged)
	return result
}

// String returns the original template string.
func (t *Template) String() string {
	return t.raw
}

// HasPlaceholders returns true if the template contains placeholders.
// Used for fast-path optimization to skip state serialization when
// no template substitution is needed.
func (t *Template) HasPlaceholders() bool {
	return t.tmpl != nil
}
