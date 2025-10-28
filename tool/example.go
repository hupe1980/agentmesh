package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/prompt"
)

// ExampleToolOptions customize how examples are rendered before being appended
// to a model request.
type ExampleToolOptions struct {
	ExamplesIntro      string
	ExamplesEnd        string
	UserPrefix         string
	AssistantPrefix    string
	Template           string
	ExamplesSeparator  string
	FunctionCallPrefix string
	FunctionCallSuffix string
}

// defaultExamplesTemplate is the fallback Go template used to render examples
// when no custom template is supplied.
const defaultExamplesTemplate = "{{$opts := .Options}}{{if .Examples}}" +
	"{{if $opts.ExamplesIntro}}{{print $opts.ExamplesIntro \"\\n\"}}{{end}}" +
	"{{range .Examples}}" +
	"{{if .Input}}" +
	"{{if $opts.UserPrefix}}{{print $opts.UserPrefix \"\\n\"}}{{end}}" +
	"{{print .Input \"\\n\"}}{{end}}" +
	"{{if .Output}}" +
	"{{if $opts.AssistantPrefix}}{{print $opts.AssistantPrefix \"\\n\"}}{{end}}" +
	"{{print .Output \"\\n\"}}{{end}}" +
	"{{if .Separator}}{{print .Separator}}{{end}}" +
	"{{end}}" +
	"{{if $opts.ExamplesEnd}}{{print $opts.ExamplesEnd \"\\n\"}}{{end}}" +
	"{{end}}"

// defaultExampleToolOptions returns the baseline ExampleToolOptions used by
// ExampleTool when no overrides are provided.
func defaultExampleToolOptions() ExampleToolOptions {
	return ExampleToolOptions{
		ExamplesIntro:      "<examples>",
		ExamplesEnd:        "</examples>",
		UserPrefix:         "[user]",
		AssistantPrefix:    "[assistant]",
		ExamplesSeparator:  "\n",
		FunctionCallPrefix: "```tool_code\n",
		FunctionCallSuffix: "\n```",
	}
}

// RenderExamples materializes the provided examples into a string using the configured options.
// It exposes both the rendered text and the original part slices to templates.
func RenderExamples(opts ExampleToolOptions, examples []core.Example) (string, error) {
	renderer := newExampleRenderer(opts)
	return renderer.render(examples)
}

// ExampleTool enriches model requests by appending rendered few-shot examples
// before execution.
type ExampleTool struct {
	provider core.ExampleProvider
	opts     ExampleToolOptions
}

// NewExampleTool constructs an ExampleTool that will source examples from the
// provided ExampleProvider and apply optional configuration overrides.
func NewExampleTool(provider core.ExampleProvider, optFns ...func(o *ExampleToolOptions)) *ExampleTool {
	opts := defaultExampleToolOptions()

	for _, fn := range optFns {
		fn(&opts)
	}

	if opts.Template == "" {
		opts.Template = defaultExamplesTemplate
	}

	return &ExampleTool{
		provider: provider,
		opts:     opts,
	}
}

// Name identifies the example tool when advertising capabilities to models.
func (t *ExampleTool) Name() string {
	return "example_tool"
}

// Description returns the summary used when advertising the tool to models.
func (t *ExampleTool) Description() string {
	return "A tool that adds (few-shot) examples to the model request."
}

// Parameters returns the tool schema exposed to the model. This tool does not
// accept arguments and therefore returns nil.
func (t *ExampleTool) Parameters() map[string]any {
	return nil
}

// IsLongRunning indicates whether the tool is a long-running operation.
func (t *ExampleTool) IsLongRunning() bool {
	return false
}

// ProcessModelRequest fetches examples, renders them, and appends the rendered
// string to the outgoing model request.
func (t *ExampleTool) ProcessModelRequest(
	ctx context.Context,
	toolCtx core.ToolContext,
	req *core.ModelRequest,
) error {
	examples, err := t.provider.Examples(ctx, toolCtx)
	if err != nil {
		return err
	}

	if len(examples) == 0 {
		return nil
	}

	rendered, err := RenderExamples(t.opts, examples)
	if err != nil {
		return err
	}

	if strings.TrimSpace(rendered) == "" {
		return nil
	}

	req.AppendInstructions(rendered)

	return nil
}

// Call is not implemented because ExampleTool only mutates outgoing requests.
func (t *ExampleTool) Call(
	ctx context.Context,
	toolCtx core.ToolContext,
	args string,
) (any, error) {
	panic("not implemented")
}

// Compile-time assertion
var _ core.Tool = (*ExampleTool)(nil)

type exampleRenderer struct {
	opts ExampleToolOptions
}

// newExampleRenderer constructs an exampleRenderer with defaults applied to the options.
func newExampleRenderer(opts ExampleToolOptions) exampleRenderer {
	if opts.Template == "" {
		opts.Template = defaultExamplesTemplate
	}

	return exampleRenderer{opts: opts}
}

// render materializes the provided examples into text using the renderer configuration.
func (r exampleRenderer) render(examples []core.Example) (string, error) {
	if len(examples) == 0 {
		return "", nil
	}

	templateExamples := make([]exampleTemplateExample, 0, len(examples))
	for idx, example := range examples {
		separator := ""
		if idx < len(examples)-1 {
			separator = r.opts.ExamplesSeparator
		}

		renderedInput, err := r.renderParts(example.Input)
		if err != nil {
			return "", fmt.Errorf("render example %d input: %w", idx+1, err)
		}

		renderedOutput, err := r.renderParts(example.Output)
		if err != nil {
			return "", fmt.Errorf("render example %d output: %w", idx+1, err)
		}

		templateExamples = append(templateExamples, exampleTemplateExample{
			Index:       idx,
			Number:      idx + 1,
			Input:       renderedInput,
			Output:      renderedOutput,
			InputParts:  example.Input,
			OutputParts: example.Output,
			Separator:   separator,
		})
	}

	rendered, err := prompt.Render(r.opts.Template, map[string]any{
		"Options":  r.opts,
		"Examples": templateExamples,
		"Count":    len(templateExamples),
	})
	if err != nil {
		return "", err
	}

	return rendered, nil
}

// renderParts joins a slice of parts into a single textual representation.
func (r exampleRenderer) renderParts(parts []core.Part) (string, error) {
	if len(parts) == 0 {
		return "", nil
	}

	var builder strings.Builder

	for i, part := range parts {
		if i > 0 {
			builder.WriteByte('\n')
		}

		rendered, err := r.renderPart(part)
		if err != nil {
			return "", err
		}

		builder.WriteString(rendered)
	}

	return builder.String(), nil
}

func (r exampleRenderer) renderPart(part core.Part) (string, error) {
	switch p := part.(type) {
	case *core.TextPart:
		return p.Text, nil
	case *core.FunctionCallPart:
		return r.renderFunctionCall(p.FunctionCall), nil
	default:
		return "", fmt.Errorf("unsupported example part type %T", part)
	}
}

func (r exampleRenderer) renderFunctionCall(fc *core.FunctionCall) string {
	if fc == nil {
		return r.wrapFunctionCall("function_call()")
	}

	args := r.renderFunctionCallArgs(fc.Arguments)
	if len(args) == 0 {
		return r.wrapFunctionCall(fmt.Sprintf("%s()", fc.Name))
	}

	return r.wrapFunctionCall(fmt.Sprintf("%s(%s)", fc.Name, strings.Join(args, ", ")))
}

func (r exampleRenderer) renderFunctionCallArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		keys := slices.Collect(maps.Keys(obj))
		slices.Sort(keys)

		args := make([]string, 0, len(keys))
		for _, key := range keys {
			args = append(args, fmt.Sprintf("%s=%s", key, r.formatArgumentValue(obj[key])))
		}

		return args
	}

	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		args := make([]string, 0, len(arr))
		for _, value := range arr {
			args = append(args, r.formatArgumentValue(value))
		}

		return args
	}

	return []string{raw}
}

func (r exampleRenderer) formatArgumentValue(value any) string {
	switch v := value.(type) {
	case string:
		return quoteSingle(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}

		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return "null"
	case map[string]any, []any:
		if encoded, err := json.Marshal(v); err == nil {
			return string(encoded)
		}
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (r exampleRenderer) wrapFunctionCall(call string) string {
	return r.opts.FunctionCallPrefix + call + r.opts.FunctionCallSuffix
}

func quoteSingle(s string) string {
	if s == "" {
		return "''"
	}

	escaped := strings.ReplaceAll(s, "'", "\\'")
	return "'" + escaped + "'"
}

type exampleTemplateExample struct {
	Index       int
	Number      int
	Input       string
	Output      string
	InputParts  []core.Part
	OutputParts []core.Part
	Separator   string
}
