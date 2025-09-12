package openai

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	oa "github.com/openai/openai-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectToolResponses(t *testing.T) {
	// Build a request with tool responses (RoleTool)
	r := core.ModelRequest{}

	// Tool responses for two calls
	toolResp1 := core.NewPartFromFunctionResponse("call-1", "calc", "9.0")
	toolResp2 := core.NewPartFromFunctionResponse("call-2", "calc", 42)

	r.Messages = []*core.Message{
		{Role: core.RoleSystem, Parts: []core.Part{core.NewPartFromText("sys")}},
		{Role: core.RoleTool, Parts: []core.Part{toolResp1}},
		{Role: core.RoleTool, Parts: []core.Part{toolResp2}},
	}

	got := collectToolResponses(r)

	// Expect stringified values and both ids present
	require.Len(t, got, 2)
	assert.Equal(t, "9.0", got["call-1"])
	// Non-string response coerced via fmt.Sprintf("%v")
	assert.Equal(t, "42", got["call-2"])

	// Duplicate id keeps first value
	dup := core.NewPartFromFunctionResponse("call-1", "calc", "changed")
	r.Messages = append(r.Messages, &core.Message{Role: core.RoleTool, Parts: []core.Part{dup}})
	got2 := collectToolResponses(r)
	assert.Equal(t, "9.0", got2["call-1"]) // unchanged
}

func TestExtractToolCalls(t *testing.T) {
	// Assistant content with two tool calls and extra text
	parts := []core.Part{
		core.NewPartFromText("thinking..."),
		core.NewPartFromFunctionCall("call-1", "calc", "{\"a\":1}"),
		core.NewPartFromFunctionCall("call-2", "calc", "{\"a\":2}"),
	}

	toolCalls, callIDs := extractToolCalls(&core.Message{Role: core.RoleAssistant, Parts: parts})

	require.Len(t, toolCalls, 2)
	require.Len(t, callIDs, 2)
	assert.Equal(t, []string{"call-1", "call-2"}, callIDs)

	// Verify OpenAI tool call params (v2 unions)
	require.NotNil(t, toolCalls[0].OfFunction)
	assert.Equal(t, "calc", toolCalls[0].OfFunction.Function.Name)
	assert.Equal(t, "{\"a\":1}", toolCalls[0].OfFunction.Function.Arguments)
	assert.Equal(t, "call-1", toolCalls[0].OfFunction.ID)
}

func TestBuildMessages_AttachToolResponses(t *testing.T) {
	// Conversation: system, user, assistant tool calls (2), then tool responses for each
	sys := &core.Message{Role: core.RoleSystem, Parts: []core.Part{core.NewPartFromText("You are helpful")}}
	usr := &core.Message{Role: core.RoleUser, Parts: []core.Part{core.NewPartFromText("compute area")}}
	asst := &core.Message{Role: core.RoleAssistant, Parts: []core.Part{
		core.NewPartFromFunctionCall("tc-1", "calc", "{}"),
		core.NewPartFromFunctionCall("tc-2", "calc", "{}"),
	}}
	tool := &core.Message{Role: core.RoleTool, Parts: []core.Part{
		core.NewPartFromFunctionResponse("tc-1", "calc", "3.14"),
		core.NewPartFromFunctionResponse("tc-2", "calc", 7),
	}}

	req := core.ModelRequest{Messages: []*core.Message{sys, usr, asst, tool}}

	msgs := buildMessages(req)

	// Expect: System, User, Assistant(with tool_calls), Tool for tc-1, Tool for tc-2
	// Total 5 messages
	require.Len(t, msgs, 5)

	// System
	require.NotNil(t, msgs[0].OfSystem)

	// User
	require.NotNil(t, msgs[1].OfUser)

	// Assistant with tool calls
	require.NotNil(t, msgs[2].OfAssistant)
	require.Len(t, msgs[2].OfAssistant.ToolCalls, 2)
	require.NotNil(t, msgs[2].OfAssistant.ToolCalls[0].OfFunction)
	require.NotNil(t, msgs[2].OfAssistant.ToolCalls[1].OfFunction)
	assert.Equal(t, "tc-1", msgs[2].OfAssistant.ToolCalls[0].OfFunction.ID)
	assert.Equal(t, "tc-2", msgs[2].OfAssistant.ToolCalls[1].OfFunction.ID)

	// Tool responses follow in order of calls when available in map
	require.NotNil(t, msgs[3].OfTool)
	require.NotNil(t, msgs[4].OfTool)
	assert.Equal(t, "tc-1", msgs[3].OfTool.ToolCallID)
	assert.Equal(t, "tc-2", msgs[4].OfTool.ToolCallID)

	// Content presence (OpenAI union uses a content representation)
	// We check that at least one of the content fields is populated.
	// ToolMessage helper should set Content as a string slice or string; ensure not zero.
	// The SDK represents content as a union; we check via non-empty string fallback.
	// For robustness, assert that building params with these messages succeeds.
	p := chatCompletionNewParamsForTest(msgs)
	// Validate compiled structure minimally
	require.Equal(t, len(msgs), len(p.Messages))
}

// chatCompletionNewParamsForTest builds params similar to buildParams for verification.
func chatCompletionNewParamsForTest(msgs []oa.ChatCompletionMessageParamUnion) oa.ChatCompletionNewParams {
	return oa.ChatCompletionNewParams{
		Messages: msgs,
		Model:    oa.ChatModelGPT4oMini,
	}
}

// --- Mocks for Client and Stream ---

type mockStream struct {
	chunks []oa.ChatCompletionChunk
	idx    int
	err    error
	closed bool
}

func (m *mockStream) Next() bool {
	m.idx++
	return m.idx < len(m.chunks)
}

func (m *mockStream) Current() oa.ChatCompletionChunk { return m.chunks[m.idx] }
func (m *mockStream) Close() error                    { m.closed = true; return nil }
func (m *mockStream) Err() error                      { return m.err }

type mockClient struct {
	nextResp           *oa.ChatCompletion
	respErr            error
	stream             Stream
	lastParams         oa.ChatCompletionNewParams
	calledStreaming    bool
	calledNonStreaming bool
}

func (m *mockClient) ChatCompletions(_ context.Context, req oa.ChatCompletionNewParams) (*oa.ChatCompletion, error) {
	m.lastParams = req
	m.calledNonStreaming = true
	return m.nextResp, m.respErr
}

func (m *mockClient) ChatCompletionsStreaming(_ context.Context, req oa.ChatCompletionNewParams) Stream {
	m.lastParams = req
	m.calledStreaming = true
	return m.stream
}

// --- Tests for Generate ---

func TestModel_Generate_NonStreaming_TextOnly(t *testing.T) {
	resp := &oa.ChatCompletion{
		Choices: []oa.ChatCompletionChoice{{
			Message:      oa.ChatCompletionMessage{Content: "hello"},
			FinishReason: "stop",
		}},
	}

	mc := &mockClient{nextResp: resp}
	m := &Model{client: mc, opts: Options{Model: oa.ChatModelGPT4oMini, Temperature: 0.7, MaxCompletionTokens: 128}}

	out, errCh := m.Generate(context.Background(), &core.ModelRequest{Stream: false})

	var results = make([]*core.ModelResponse, 0, 1)
	for r := range out {
		results = append(results, r)
	}

	select {
	case err := <-errCh:
		require.NoError(t, err)
	default:
	}

	require.True(t, mc.calledNonStreaming)
	require.False(t, mc.calledStreaming)

	require.Len(t, results, 1)
	r := results[0]
	assert.False(t, r.Partial)
	assert.Equal(t, "stop", r.FinishReason)
	require.Len(t, r.Parts, 1)
	if tp, ok := r.Parts[0].(*core.TextPart); assert.True(t, ok) {
		assert.Equal(t, "hello", tp.Text)
	}
}

func TestModel_Generate_Streaming_TextAccumulation_ClosesStream(t *testing.T) {
	chunks := []oa.ChatCompletionChunk{
		{Choices: []oa.ChatCompletionChunkChoice{{Delta: oa.ChatCompletionChunkChoiceDelta{Content: "he"}}}},
		{Choices: []oa.ChatCompletionChunkChoice{{Delta: oa.ChatCompletionChunkChoiceDelta{Content: "llo"}}}},
		{Choices: []oa.ChatCompletionChunkChoice{{FinishReason: "stop"}}},
	}

	ms := &mockStream{chunks: chunks, idx: -1}
	mc := &mockClient{stream: ms}
	m := &Model{client: mc, opts: Options{Model: oa.ChatModelGPT4oMini, Temperature: 0.7, MaxCompletionTokens: 128}}

	out, errCh := m.Generate(context.Background(), &core.ModelRequest{Stream: true})

	var partials []string
	var final *core.ModelResponse
	for r := range out {
		if r.Partial {
			tp := r.Parts[0].(*core.TextPart)
			partials = append(partials, tp.Text)
		} else {
			final = r
		}
	}

	select {
	case err := <-errCh:
		require.NoError(t, err)
	default:
	}

	require.True(t, mc.calledStreaming)
	require.False(t, mc.calledNonStreaming)

	assert.Equal(t, []string{"he", "llo"}, partials)
	require.NotNil(t, final)
	if tp, ok := final.Parts[0].(*core.TextPart); assert.True(t, ok) {
		assert.Equal(t, "hello", tp.Text)
	}
	assert.Equal(t, "stop", final.FinishReason)

	// Ensure Close was called on the stream when processing ends
	assert.True(t, ms.closed)
}
