package guardrail

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContentFilterGuardrail(t *testing.T) {
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		g := NewContentFilterGuardrail([]string{"bad"})
		assert.Equal(t, "content_filter", g.Name())
	})

	t.Run("AllowsCleanContent", func(t *testing.T) {
		g := NewContentFilterGuardrail([]string{"bad", "evil", "harmful"})
		result, err := g.Check(ctx, "This is a nice message")

		assert.NoError(t, err)
		assert.Equal(t, ActionAllow, result.Action)
	})

	t.Run("RejectsBlockedKeyword", func(t *testing.T) {
		g := NewContentFilterGuardrail([]string{"bad", "evil"})
		result, err := g.Check(ctx, "This is a bad message")

		assert.NoError(t, err)
		assert.Equal(t, ActionReject, result.Action)
		assert.Contains(t, result.Message, "bad")
	})

	t.Run("CaseInsensitiveByDefault", func(t *testing.T) {
		g := NewContentFilterGuardrail([]string{"BAD"})
		result, err := g.Check(ctx, "this is a bad message")

		assert.NoError(t, err)
		assert.Equal(t, ActionReject, result.Action)
	})

	t.Run("CaseSensitive", func(t *testing.T) {
		g := NewContentFilterGuardrail([]string{"BAD"}, WithCaseSensitive(true))

		// Should allow lowercase
		result1, err := g.Check(ctx, "this is a bad message")
		assert.NoError(t, err)
		assert.Equal(t, ActionAllow, result1.Action)

		// Should reject uppercase
		result2, err := g.Check(ctx, "this is a BAD message")
		assert.NoError(t, err)
		assert.Equal(t, ActionReject, result2.Action)
	})

	t.Run("RaisesWhenConfigured", func(t *testing.T) {
		g := NewContentFilterGuardrail([]string{"bad"}, WithContentFilterAction(ActionRaise))
		result, err := g.Check(ctx, "bad content")

		assert.NoError(t, err)
		assert.Equal(t, ActionRaise, result.Action)
	})

	t.Run("MatchedKeywordInInfo", func(t *testing.T) {
		g := NewContentFilterGuardrail([]string{"forbidden"})
		result, err := g.Check(ctx, "This contains forbidden content")

		assert.NoError(t, err)
		info, ok := result.Info.(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, "forbidden", info["matched_keyword"])
	})
}

func TestLengthGuardrail(t *testing.T) {
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		g := NewLengthGuardrail()
		assert.Equal(t, "length_validation", g.Name())
	})

	t.Run("AllowsContentWithinLimits", func(t *testing.T) {
		g := NewLengthGuardrail(WithMinLength(5), WithMaxLength(100))
		result, err := g.Check(ctx, "This is fine")

		assert.NoError(t, err)
		assert.Equal(t, ActionAllow, result.Action)
	})

	t.Run("RejectsTooShort", func(t *testing.T) {
		g := NewLengthGuardrail(WithMinLength(10))
		result, err := g.Check(ctx, "short")

		assert.NoError(t, err)
		assert.Equal(t, ActionReject, result.Action)
		assert.Contains(t, result.Message, "too short")
	})

	t.Run("RejectsTooLong", func(t *testing.T) {
		g := NewLengthGuardrail(WithMaxLength(5))
		result, err := g.Check(ctx, "This is too long")

		assert.NoError(t, err)
		assert.Equal(t, ActionReject, result.Action)
		assert.Contains(t, result.Message, "too long")
	})

	t.Run("NoLimitsAllowsAll", func(t *testing.T) {
		g := NewLengthGuardrail()

		result1, _ := g.Check(ctx, "")
		assert.Equal(t, ActionAllow, result1.Action)

		result2, _ := g.Check(ctx, "A very long message that should still pass")
		assert.Equal(t, ActionAllow, result2.Action)
	})

	t.Run("RaisesWhenConfigured", func(t *testing.T) {
		g := NewLengthGuardrail(WithMaxLength(5), WithLengthAction(ActionRaise))
		result, err := g.Check(ctx, "Too long content")

		assert.NoError(t, err)
		assert.Equal(t, ActionRaise, result.Action)
	})

	t.Run("LengthInInfo", func(t *testing.T) {
		g := NewLengthGuardrail(WithMinLength(100))
		result, err := g.Check(ctx, "short")

		assert.NoError(t, err)
		info, ok := result.Info.(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, 5, info["length"])
		assert.Equal(t, 100, info["min_length"])
	})
}

func TestRegexGuardrail(t *testing.T) {
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		pattern := regexp.MustCompile(`test`)
		g := NewRegexGuardrail("custom-regex", pattern)
		assert.Equal(t, "custom-regex", g.Name())
	})

	t.Run("BlocksMatchingPattern", func(t *testing.T) {
		// Block content containing numbers
		pattern := regexp.MustCompile(`\d+`)
		g := NewRegexGuardrail("no-numbers", pattern)
		result, err := g.Check(ctx, "There are 123 numbers here")

		assert.NoError(t, err)
		assert.Equal(t, ActionReject, result.Action)
	})

	t.Run("AllowsNonMatchingPattern", func(t *testing.T) {
		pattern := regexp.MustCompile(`\d+`)
		g := NewRegexGuardrail("no-numbers", pattern)
		result, err := g.Check(ctx, "No numbers here")

		assert.NoError(t, err)
		assert.Equal(t, ActionAllow, result.Action)
	})

	t.Run("MustMatchMode", func(t *testing.T) {
		// Require content to match email pattern
		pattern := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
		g := NewRegexGuardrail("email-required", pattern, WithMustMatch(true))

		// Should reject non-email
		result1, _ := g.Check(ctx, "not an email")
		assert.Equal(t, ActionReject, result1.Action)

		// Should allow email
		result2, _ := g.Check(ctx, "test@example.com")
		assert.Equal(t, ActionAllow, result2.Action)
	})

	t.Run("RaisesWhenConfigured", func(t *testing.T) {
		pattern := regexp.MustCompile(`forbidden`)
		g := NewRegexGuardrail("block-forbidden", pattern, WithRegexAction(ActionRaise))
		result, err := g.Check(ctx, "This is forbidden content")

		assert.NoError(t, err)
		assert.Equal(t, ActionRaise, result.Action)
	})

	t.Run("CustomDescription", func(t *testing.T) {
		pattern := regexp.MustCompile(`secret`)
		g := NewRegexGuardrail("block-secret", pattern, WithDescription("No secrets allowed!"))
		result, err := g.Check(ctx, "This contains a secret")

		assert.NoError(t, err)
		assert.Equal(t, "No secrets allowed!", result.Message)
	})

	t.Run("PatternInInfo", func(t *testing.T) {
		pattern := regexp.MustCompile(`test`)
		g := NewRegexGuardrail("test-pattern", pattern)
		result, err := g.Check(ctx, "test content")

		assert.NoError(t, err)
		info, ok := result.Info.(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, "test", info["pattern"])
		assert.Equal(t, false, info["must_match"])
		assert.Equal(t, true, info["matched"])
	})
}

func TestGuardrail_Interface(t *testing.T) {
	// Verify all built-in guardrails implement Guardrail[string]
	var _ Guardrail[string] = (*ContentFilterGuardrail)(nil)
	var _ Guardrail[string] = (*LengthGuardrail)(nil)
	var _ Guardrail[string] = (*RegexGuardrail)(nil)
	var _ Guardrail[string] = (*ChainGuardrail[string])(nil)
	var _ Guardrail[string] = (*AnyGuardrail[string])(nil)
}
