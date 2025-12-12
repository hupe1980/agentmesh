package guardrail

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ContentFilterGuardrail blocks content containing specified keywords.
// Implements Guardrail[string].
type ContentFilterGuardrail struct {
	blockedKeywords []string
	action          Action // ActionReject or ActionRaise
	caseSensitive   bool
}

// ContentFilterOption configures a ContentFilterGuardrail.
type ContentFilterOption func(*ContentFilterGuardrail)

// WithContentFilterAction sets the action when blocked content is detected.
// Default is ActionReject (soft rejection).
func WithContentFilterAction(action Action) ContentFilterOption {
	return func(g *ContentFilterGuardrail) {
		g.action = action
	}
}

// WithCaseSensitive enables case-sensitive matching.
func WithCaseSensitive(sensitive bool) ContentFilterOption {
	return func(g *ContentFilterGuardrail) {
		g.caseSensitive = sensitive
	}
}

// NewContentFilterGuardrail creates a guardrail that blocks specified keywords.
func NewContentFilterGuardrail(keywords []string, opts ...ContentFilterOption) *ContentFilterGuardrail {
	g := &ContentFilterGuardrail{
		blockedKeywords: keywords,
		action:          ActionReject, // Default: soft rejection
		caseSensitive:   false,
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

// Name returns the guardrail name.
func (g *ContentFilterGuardrail) Name() string { return "content_filter" }

// Check validates the content for blocked keywords.
func (g *ContentFilterGuardrail) Check(_ context.Context, content string) (*Result, error) {
	checkContent := content
	if !g.caseSensitive {
		checkContent = strings.ToLower(content)
	}

	for _, keyword := range g.blockedKeywords {
		checkKeyword := keyword
		if !g.caseSensitive {
			checkKeyword = strings.ToLower(keyword)
		}

		if strings.Contains(checkContent, checkKeyword) {
			info := map[string]any{"matched_keyword": keyword}

			switch g.action {
			case ActionRaise:
				return RaiseWithInfo(
					fmt.Sprintf("Blocked content detected: %s", keyword), info), nil
			default:
				return RejectWithInfo(
					fmt.Sprintf("Content contains blocked keyword (%s)", keyword), info), nil
			}
		}
	}

	return Allow(), nil
}

// LengthGuardrail validates content length.
// Implements Guardrail[string].
type LengthGuardrail struct {
	minLength int
	maxLength int
	action    Action
}

// LengthGuardrailOption configures a LengthGuardrail.
type LengthGuardrailOption func(*LengthGuardrail)

// WithLengthAction sets the action when length validation fails.
func WithLengthAction(action Action) LengthGuardrailOption {
	return func(g *LengthGuardrail) {
		g.action = action
	}
}

// WithMinLength sets the minimum length.
func WithMinLength(minLen int) LengthGuardrailOption {
	return func(g *LengthGuardrail) {
		g.minLength = minLen
	}
}

// WithMaxLength sets the maximum length.
func WithMaxLength(maxLen int) LengthGuardrailOption {
	return func(g *LengthGuardrail) {
		g.maxLength = maxLen
	}
}

// NewLengthGuardrail creates a guardrail that validates content length.
func NewLengthGuardrail(opts ...LengthGuardrailOption) *LengthGuardrail {
	g := &LengthGuardrail{
		minLength: 0,
		maxLength: 0, // 0 means no limit
		action:    ActionReject,
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

// Name returns the guardrail name.
func (g *LengthGuardrail) Name() string { return "length_validation" }

// Check validates the content length.
func (g *LengthGuardrail) Check(_ context.Context, content string) (*Result, error) {
	length := len(content)

	if g.minLength > 0 && length < g.minLength {
		info := map[string]any{"length": length, "min_length": g.minLength}

		switch g.action {
		case ActionRaise:
			return RaiseWithInfo(
				fmt.Sprintf("Content too short: %d < %d", length, g.minLength), info), nil
		default:
			return RejectWithInfo(
				fmt.Sprintf("Content too short: %d < %d", length, g.minLength), info), nil
		}
	}

	if g.maxLength > 0 && length > g.maxLength {
		info := map[string]any{"length": length, "max_length": g.maxLength}

		switch g.action {
		case ActionRaise:
			return RaiseWithInfo(
				fmt.Sprintf("Content too long: %d > %d", length, g.maxLength), info), nil
		default:
			return RejectWithInfo(
				fmt.Sprintf("Content too long: %d > %d", length, g.maxLength), info), nil
		}
	}

	return Allow(), nil
}

// RegexGuardrail validates content against a regex pattern.
// Implements Guardrail[string].
type RegexGuardrail struct {
	name        string
	pattern     *regexp.Regexp
	action      Action
	mustMatch   bool // If true, content MUST match; if false, content must NOT match
	description string
}

// RegexGuardrailOption configures a RegexGuardrail.
type RegexGuardrailOption func(*RegexGuardrail)

// WithRegexAction sets the action when validation fails.
func WithRegexAction(action Action) RegexGuardrailOption {
	return func(g *RegexGuardrail) {
		g.action = action
	}
}

// WithMustMatch sets whether content must match (true) or must not match (false).
func WithMustMatch(mustMatch bool) RegexGuardrailOption {
	return func(g *RegexGuardrail) {
		g.mustMatch = mustMatch
	}
}

// WithDescription sets a custom description for the guardrail.
func WithDescription(desc string) RegexGuardrailOption {
	return func(g *RegexGuardrail) {
		g.description = desc
	}
}

// NewRegexGuardrail creates a guardrail that validates against a regex.
func NewRegexGuardrail(name string, pattern *regexp.Regexp, opts ...RegexGuardrailOption) *RegexGuardrail {
	g := &RegexGuardrail{
		name:        name,
		pattern:     pattern,
		action:      ActionReject,
		mustMatch:   false, // Default: content must NOT match (blocking pattern)
		description: "pattern validation failed",
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

// Name returns the guardrail name.
func (g *RegexGuardrail) Name() string { return g.name }

// Check validates the content against the regex.
func (g *RegexGuardrail) Check(_ context.Context, content string) (*Result, error) {
	matches := g.pattern.MatchString(content)

	// If mustMatch is true, content should match; if false, content should NOT match
	valid := (g.mustMatch && matches) || (!g.mustMatch && !matches)

	if !valid {
		info := map[string]any{
			"pattern":    g.pattern.String(),
			"must_match": g.mustMatch,
			"matched":    matches,
		}

		switch g.action {
		case ActionRaise:
			return RaiseWithInfo(g.description, info), nil
		default:
			return RejectWithInfo(g.description, info), nil
		}
	}

	return Allow(), nil
}
