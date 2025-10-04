package prompt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRender_NoTemplate(t *testing.T) {
	out, err := Render("plain text", map[string]any{"foo": "bar"})
	require.NoError(t, err)
	require.Equal(t, "plain text", out)
}

func TestRender_WithFuncs(t *testing.T) {
	tpl := `Hello {{ upper .name }}! Default: {{ default "unknown" .missing }} Join: {{ join "," .items }}`
	out, err := Render(tpl, map[string]any{
		"name":    "world",
		"items":   []any{"a", "b"},
		"missing": "",
	})
	require.NoError(t, err)
	require.Equal(t, "Hello WORLD! Default: unknown Join: a,b", out)
}
