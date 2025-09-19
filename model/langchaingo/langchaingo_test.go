package langchaingo

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

// mockModel implements llms.Model for testing the adapter behavior.
type mockModel struct {
	CapturedMessages []llms.MessageContent
	CapturedTools    []llms.Tool
	StreamChunks     []string
	FinalContent     string
	StopReason       string
	Err              error
}

func (m *mockModel) GenerateContent(
	ctx context.Context,
	messages []llms.MessageContent,
	options ...llms.CallOption,
) (*llms.ContentResponse, error) {
	// Capture input
	m.CapturedMessages = append([]llms.MessageContent(nil), messages...)

	// Apply options
	var opts llms.CallOptions
	for _, opt := range options {
		opt(&opts)
	}
	// Capture tools
	if len(opts.Tools) > 0 {
		m.CapturedTools = append([]llms.Tool(nil), opts.Tools...)
	}

	// Simulate streaming if requested
	if opts.StreamingFunc != nil && len(m.StreamChunks) > 0 {
		for _, ch := range m.StreamChunks {
			if err := opts.StreamingFunc(ctx, []byte(ch)); err != nil {
				return nil, err
			}
		}
	}

	if m.Err != nil {
		return nil, m.Err
	}

	// Build default response when none provided
	if m.StopReason == "" {
		m.StopReason = "stop"
	}

	resp := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{
			Content:        m.FinalContent,
			StopReason:     m.StopReason,
			ToolCalls:      nil,
			FuncCall:       nil,
			GenerationInfo: map[string]any{},
		}},
	}

	return resp, nil
}

func (m *mockModel) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	resp, err := m.GenerateContent(
		ctx,
		[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, prompt)},
		options...,
	)
	if err != nil {
		return "", err
	}

	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0] == nil {
		return "", fmt.Errorf("empty response from model")
	}

	return resp.Choices[0].Content, nil
}

func TestGenerate_NonStreaming_SendsFinalAndMapsMessages(t *testing.T) {
	mm := &mockModel{FinalContent: "Final content", StopReason: "length"}
	m, err := NewModel(mm)
	require.NoError(t, err)

	req := &core.ModelRequest{
		Instructions: "You are helpful",
		Messages: []*core.Message{
			{Role: core.RoleUser, Parts: []core.Part{&core.TextPart{Text: "Hello"}}},
			{Role: core.RoleAssistant, Parts: []core.Part{&core.TextPart{Text: "Hi"}}},
			{Role: core.RoleTool, Parts: []core.Part{&core.TextPart{Text: "tool says ok"}}},
			{Role: core.RoleUser, Parts: []core.Part{
				&core.TextPart{Text: "continue"},
				&core.DataPart{Data: map[string]any{"k": "v"}},
			}},
		},
	}

	ctx := context.Background()
	outCh, errCh := m.Generate(ctx, req)

	resps, errs := drain(outCh, errCh)
	require.Len(t, errs, 0)
	require.Len(t, resps, 1)

	final := resps[0]
	assert.False(t, final.Partial)
	require.Len(t, final.Parts, 1)
	if tp, ok := final.Parts[0].(*core.TextPart); assert.True(t, ok) {
		assert.Equal(t, "Final content", tp.Text)
	}
	assert.Equal(t, "length", final.FinishReason)

	// Verify messages mapped correctly to roles and content
	require.GreaterOrEqual(t, len(mm.CapturedMessages), 5)
	roles := make([]llms.ChatMessageType, 0, len(mm.CapturedMessages))
	contents := make([]string, 0, len(mm.CapturedMessages))
	for _, mc := range mm.CapturedMessages {
		roles = append(roles, mc.Role)
		// Expect a single text part in our adapter
		require.Len(t, mc.Parts, 1)
		if tc, ok := mc.Parts[0].(llms.TextContent); assert.True(t, ok) {
			contents = append(contents, tc.Text)
		}
	}

	assert.Equal(t, []llms.ChatMessageType{
		llms.ChatMessageTypeSystem,
		llms.ChatMessageTypeHuman,
		llms.ChatMessageTypeAI,
		llms.ChatMessageTypeTool,
		llms.ChatMessageTypeHuman,
	}, roles[:5])
	assert.Equal(t, []string{"You are helpful", "Hello", "Hi", "tool says ok", "continue"}, contents[:5])
}

func TestGenerate_Streaming_EmitsPartialsAndFinal(t *testing.T) {
	mm := &mockModel{StreamChunks: []string{"Hel", "lo"}, FinalContent: "Hello", StopReason: "stop"}
	m, err := NewModel(mm)
	require.NoError(t, err)

	req := &core.ModelRequest{
		Instructions: "System",
		Messages:     []*core.Message{{Role: core.RoleUser, Parts: []core.Part{&core.TextPart{Text: "Say hi"}}}},
		Stream:       true,
	}

	ctx := context.Background()
	outCh, errCh := m.Generate(ctx, req)

	resps, errs := drain(outCh, errCh)
	require.Len(t, errs, 0)
	require.Len(t, resps, 3)

	// Two partial chunks, then final
	assert.True(t, resps[0].Partial)
	assert.True(t, resps[1].Partial)
	assert.False(t, resps[2].Partial)

	require.Len(t, resps[0].Parts, 1)
	require.Len(t, resps[1].Parts, 1)
	require.Len(t, resps[2].Parts, 1)

	assert.Equal(t, "Hel", mustText(t, resps[0].Parts[0]))
	assert.Equal(t, "lo", mustText(t, resps[1].Parts[0]))
	assert.Equal(t, "Hello", mustText(t, resps[2].Parts[0]))
	assert.Equal(t, "stop", resps[2].FinishReason)
}

func TestGenerate_EmptyPrompt_Error(t *testing.T) {
	mm := &mockModel{}
	m, err := NewModel(mm)
	require.NoError(t, err)

	req := &core.ModelRequest{ // no instructions and only non-text parts
		Messages: []*core.Message{{Role: core.RoleUser, Parts: []core.Part{&core.DataPart{Data: map[string]any{"a": 1}}}}},
	}
	ctx := context.Background()
	outCh, errCh := m.Generate(ctx, req)

	resps, errs := drain(outCh, errCh)
	require.Len(t, resps, 0)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "empty prompt")
}

func TestGenerate_LLMError_Propagates(t *testing.T) {
	mm := &mockModel{Err: fmt.Errorf("boom")}
	m, err := NewModel(mm)
	require.NoError(t, err)

	req := &core.ModelRequest{
		Instructions: "Sys",
		Messages:     []*core.Message{{Role: core.RoleUser, Parts: []core.Part{&core.TextPart{Text: "Hi"}}}},
	}
	ctx := context.Background()
	outCh, errCh := m.Generate(ctx, req)

	resps, errs := drain(outCh, errCh)
	require.Len(t, resps, 0)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "boom")
}

func TestGenerate_WithTools_MapsDefinitions(t *testing.T) {
	mm := &mockModel{FinalContent: "ok"}
	m, err := NewModel(mm)
	require.NoError(t, err)

	req := &core.ModelRequest{
		Instructions: "Sys",
		Messages:     []*core.Message{{Role: core.RoleUser, Parts: []core.Part{&core.TextPart{Text: "Hi"}}}},
		Tools: []core.ToolDefinition{{
			Type: "function",
			Function: core.FunctionDefinition{
				Name:        "get_weather",
				Description: "Get the weather",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{
							"type": "string",
						},
					},
				},
			},
		}},
	}

	ctx := context.Background()
	outCh, errCh := m.Generate(ctx, req)
	_, errs := drain(outCh, errCh)
	require.Len(t, errs, 0)

	require.Len(t, mm.CapturedTools, 1)
	tool := mm.CapturedTools[0]
	require.NotNil(t, tool.Function)
	assert.Equal(t, "function", tool.Type)
	assert.Equal(t, "get_weather", tool.Function.Name)
	assert.Equal(t, "Get the weather", tool.Function.Description)
	// Spot-check parameters mapping
	require.IsType(t, map[string]any{}, tool.Function.Parameters)
	params := tool.Function.Parameters.(map[string]any)
	assert.Equal(t, "object", params["type"])
}

func TestBuildMessages_AttachToolResponses_AfterAssistant(t *testing.T) {
	// Conversation: instructions, user, assistant tool calls (2), then tool responses for each
	mm := &mockModel{FinalContent: "ok"}
	m, err := NewModel(mm)
	require.NoError(t, err)

	asst := &core.Message{Role: core.RoleAssistant, Parts: []core.Part{
		core.NewPartFromFunctionCall("tc-1", "calc", "{}"),
		core.NewPartFromFunctionCall("tc-2", "calc", "{}"),
	}}
	tool := &core.Message{Role: core.RoleTool, Parts: []core.Part{
		core.NewPartFromFunctionResponse("tc-1", "calc", "3.14"),
		core.NewPartFromFunctionResponse("tc-2", "calc", 7),
	}}

	req := &core.ModelRequest{
		Instructions: "You are helpful",
		Messages: []*core.Message{
			{Role: core.RoleUser, Parts: []core.Part{core.NewPartFromText("compute area")}},
			asst,
			tool,
		},
	}

	ctx := context.Background()
	outCh, errCh := m.Generate(ctx, req)
	// Drain to ensure Generate runs fully
	_, errs := drain(outCh, errCh)
	require.Len(t, errs, 0)

	// Expect: System, User, Assistant (with 2 ToolCall parts), Tool (response 1), Tool (response 2)
	require.GreaterOrEqual(t, len(mm.CapturedMessages), 5)

	// System
	m0 := mm.CapturedMessages[0]
	assert.Equal(t, llms.ChatMessageTypeSystem, m0.Role)
	require.Len(t, m0.Parts, 1)
	if tc, ok := m0.Parts[0].(llms.TextContent); assert.True(t, ok) {
		assert.Equal(t, "You are helpful", tc.Text)
	}

	// User
	m1 := mm.CapturedMessages[1]
	assert.Equal(t, llms.ChatMessageTypeHuman, m1.Role)
	require.Len(t, m1.Parts, 1)
	if tc, ok := m1.Parts[0].(llms.TextContent); assert.True(t, ok) {
		assert.Equal(t, "compute area", tc.Text)
	}

	// Assistant with ToolCalls
	m2 := mm.CapturedMessages[2]
	assert.Equal(t, llms.ChatMessageTypeAI, m2.Role)
	require.Len(t, m2.Parts, 2)
	if tc, ok := m2.Parts[0].(llms.ToolCall); assert.True(t, ok) {
		assert.Equal(t, "tc-1", tc.ID)
		require.NotNil(t, tc.FunctionCall)
		assert.Equal(t, "calc", tc.FunctionCall.Name)
	}
	if tc, ok := m2.Parts[1].(llms.ToolCall); assert.True(t, ok) {
		assert.Equal(t, "tc-2", tc.ID)
		require.NotNil(t, tc.FunctionCall)
		assert.Equal(t, "calc", tc.FunctionCall.Name)
	}

	// Tool response for tc-1
	m3 := mm.CapturedMessages[3]
	assert.Equal(t, llms.ChatMessageTypeTool, m3.Role)
	require.Len(t, m3.Parts, 1)
	if tr, ok := m3.Parts[0].(llms.ToolCallResponse); assert.True(t, ok) {
		assert.Equal(t, "tc-1", tr.ToolCallID)
		assert.Equal(t, "calc", tr.Name)
		assert.Equal(t, "3.14", tr.Content)
	}

	// Tool response for tc-2
	m4 := mm.CapturedMessages[4]
	assert.Equal(t, llms.ChatMessageTypeTool, m4.Role)
	require.Len(t, m4.Parts, 1)
	if tr, ok := m4.Parts[0].(llms.ToolCallResponse); assert.True(t, ok) {
		assert.Equal(t, "tc-2", tr.ToolCallID)
		assert.Equal(t, "calc", tr.Name)
		assert.Equal(t, "7", tr.Content)
	}
}

// Helpers
func drain(outCh <-chan *core.ModelResponse, errCh <-chan error) (resps []*core.ModelResponse, errs []error) {
	for outCh != nil || errCh != nil {
		select {
		case r, ok := <-outCh:
			if !ok {
				outCh = nil
				continue
			}
			resps = append(resps, r)
		case e, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			errs = append(errs, e)
		}
	}
	return resps, errs
}

func mustText(t *testing.T, p core.Part) string {
	t.Helper()
	if tp, ok := p.(*core.TextPart); ok {
		return tp.Text
	}
	require.Fail(t, "part is not TextPart")
	return ""
}
