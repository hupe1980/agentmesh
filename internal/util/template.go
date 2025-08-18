package util

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// RenderTemplate replaces template variables using Go's text/template package.
// This lives in internal to avoid committing to public API stability prematurely.
func RenderTemplate(text string, state map[string]any) (string, error) {
	if !strings.Contains(text, "{{") { // fast path: no template markers
		return text, nil
	}

	// Create a new template with helper funcs
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
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(string(s[0])) + strings.ToLower(s[1:])
		},
		"join": func(sep string, items []interface{}) string {
			strItems := make([]string, len(items))
			for i, item := range items {
				strItems[i] = fmt.Sprintf("%v", item)
			}
			return strings.Join(strItems, sep)
		},
	}).Parse(text)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, state); err != nil {
		return "", err
	}

	return buf.String(), nil
}
