package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParts_DiscriminatedUnion(t *testing.T) {
	parts := []Part{
		&TextPart{Text: "hello"},
		&DataPart{Data: map[string]any{"k": "v"}},
		&FilePart{File: FileURI{URI: "file://x"}},
		&FilePart{File: FilePath{Path: "/tmp/file.txt"}},
		&FilePart{File: FileBase64{Base64: "Zm9v"}},
		&FilePart{File: FileRawBytes{Bytes: []byte("bar")}},
		&FunctionCallPart{FunctionCall: &FunctionCall{Name: "f"}},
		&FunctionResponsePart{FunctionResponse: &FunctionResponse{Name: "f"}},
	}
	for _, p := range parts {
		switch pt := p.(type) {
		case *TextPart, *DataPart, *FilePart, *FunctionCallPart, *FunctionResponsePart:
			// expected
		default:
			require.Failf(t, "unexpected part type", "%T (%v)", pt, pt)
		}
	}
}

func TestNewPartConstructors(t *testing.T) {
	// NewPartFromText
	p := NewPartFromText("hi")
	tp, ok := p.(*TextPart)
	require.True(t, ok)
	assert.Equal(t, "hi", tp.Text)

	// NewPartFromFunctionCall
	p2 := NewPartFromFunctionCall("id1", "do", "{}")
	fcp, ok := p2.(*FunctionCallPart)
	require.True(t, ok)
	assert.Equal(t, "id1", fcp.FunctionCall.ID)
	assert.Equal(t, "do", fcp.FunctionCall.Name)
	assert.Equal(t, "{}", fcp.FunctionCall.Arguments)

	// NewPartFromFunctionResponse success
	p3 := NewPartFromFunctionResponse("id2", "do", 42)
	frp, ok := p3.(*FunctionResponsePart)
	require.True(t, ok)
	assert.Equal(t, "id2", frp.FunctionResponse.ID)
	assert.Equal(t, "do", frp.FunctionResponse.Name)
	require.NotNil(t, frp.FunctionResponse.Response)
	assert.Equal(t, 42, frp.FunctionResponse.Response.(int))

	// NewPartFromFunctionResponse error
	p4 := NewPartFromFunctionResponse("id3", "do", nil)
	frp2, ok := p4.(*FunctionResponsePart)
	require.True(t, ok)
	assert.Equal(t, "id3", frp2.FunctionResponse.ID)
	assert.Equal(t, "do", frp2.FunctionResponse.Name)
}
