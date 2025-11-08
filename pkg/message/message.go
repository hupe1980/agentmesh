package message

import "fmt"

// Part is a polymorphic segment of role-based content. Concrete part types
// implement the unexported isPart marker to form a closed set.
type Part interface{ isPart() }

// Parts represents an ordered sequence of content parts.
type Parts = []Part

// Type indicates the conversational role/category associated with a message.
type Type string

const (
	TypeSystem   Type = "system"
	TypeHuman    Type = "human"
	TypeAI       Type = "ai"
	TypeChat     Type = "chat"
	TypeFunction Type = "function"
	TypeTool     Type = "tool"
)

// TextPart is a plain UTF-8 text content segment.
type TextPart struct {
	Text string
}

func (TextPart) isPart() {}

// DataPart is a structured data segment (for example a JSON object map).
type DataPart struct {
	Data map[string]any
}

func (DataPart) isPart() {}

// FilePart represents a file segment within a message or artifact.
type FilePart struct {
	File     FilePartContent
	MimeType string
	Name     string
}

func (FilePart) isPart() {}

// FilePartContent is a discriminated union representing file payloads.
type FilePartContent interface{ isFilePartContent() }

// FileRawBytes represents a file with its content provided directly as raw bytes.
type FileRawBytes struct {
	Bytes []byte
}

// FileBase64 represents a file with its content provided as a base64-encoded string.
type FileBase64 struct {
	Base64 string
}

// FilePath represents a local filesystem path to a file.
type FilePath struct {
	Path string
}

// FileURI represents a file whose content is available via URI.
type FileURI struct {
	URI string
}

func (FileRawBytes) isFilePartContent() {}
func (FileBase64) isFilePartContent()   {}
func (FilePath) isFilePartContent()     {}
func (FileURI) isFilePartContent()      {}

// FunctionCall describes a tool/function invocation request.
type FunctionCall struct {
	ID        string `json:"id,omitzero"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitzero"`
}

// FunctionCallPart wraps a FunctionCall as a content part.
type FunctionCallPart struct {
	FunctionCall *FunctionCall
}

func (FunctionCallPart) isPart() {}

// FunctionResponse describes the outcome of a function call.
type FunctionResponse struct {
	ID       string `json:"id,omitzero"`
	Name     string `json:"name"`
	Response any    `json:"response,omitzero"`
}

// FunctionResponsePart wraps a FunctionResponse as a content part.
type FunctionResponsePart struct {
	FunctionResponse *FunctionResponse
}

func (FunctionResponsePart) isPart() {}

// ToolCall mirrors LangChain tool invocation metadata.
type ToolCall struct {
	ID        string
	Name      string
	Type      string
	Arguments map[string]any
}

// Message represents the minimal shape shared across message variants.
type Message interface {
	Type() Type
	Parts() Parts
	Clone() Message
}

// Option configures creation of messageBase derivatives.
type Option func(*messageBase)

type messageBase struct {
	content Parts
}

func newMessageBase(content Parts, opts ...Option) messageBase {
	base := messageBase{content: cloneParts(content)}
	for _, opt := range opts {
		if opt != nil {
			opt(&base)
		}
	}
	return base
}

func (b messageBase) clone() messageBase {
	return messageBase{content: cloneParts(b.content)}
}

func (b messageBase) contentClone() Parts {
	return cloneParts(b.content)
}

// SystemMessage models a system-role message.
type SystemMessage struct {
	base messageBase
}

func NewSystemMessage(parts Parts, opts ...Option) *SystemMessage {
	return &SystemMessage{base: newMessageBase(parts, opts...)}
}

func NewSystemMessageFromText(text string, opts ...Option) *SystemMessage {
	return NewSystemMessage(partsFromText(text), opts...)
}

func (m *SystemMessage) Type() Type { return TypeSystem }

func (m *SystemMessage) Parts() Parts {
	return m.base.contentClone()
}

func (m *SystemMessage) Clone() Message {
	clone := *m
	clone.base = m.base.clone()
	return &clone
}

// HumanMessage models a human/user authored message.
type HumanMessage struct {
	base messageBase
}

func NewHumanMessage(parts Parts, opts ...Option) *HumanMessage {
	return &HumanMessage{base: newMessageBase(parts, opts...)}
}

func NewHumanMessageFromText(text string, opts ...Option) *HumanMessage {
	return NewHumanMessage(partsFromText(text), opts...)
}

func (m *HumanMessage) Type() Type { return TypeHuman }

func (m *HumanMessage) Parts() Parts {
	return m.base.contentClone()
}

func (m *HumanMessage) Clone() Message {
	clone := *m
	clone.base = m.base.clone()
	return &clone
}

// AIMessage models an assistant/AI response.
type AIMessage struct {
	base      messageBase
	ToolCalls []ToolCall
	Name      string
}

func NewAIMessage(parts Parts, opts ...Option) *AIMessage {
	return &AIMessage{base: newMessageBase(parts, opts...)}
}

func NewAIMessageFromText(text string, opts ...Option) *AIMessage {
	return NewAIMessage(partsFromText(text), opts...)
}

func (m *AIMessage) Type() Type { return TypeAI }

func (m *AIMessage) Parts() Parts {
	return m.base.contentClone()
}

func (m *AIMessage) Clone() Message {
	clone := *m
	clone.base = m.base.clone()
	clone.ToolCalls = cloneToolCalls(m.ToolCalls)
	return &clone
}

// ChatMessage models a custom-role chat message.
type ChatMessage struct {
	base    messageBase
	msgType Type
}

func NewChatMessage(role string, text string, opts ...Option) *ChatMessage {
	return &ChatMessage{base: newMessageBase(partsFromText(text), opts...), msgType: Type(role)}
}

func (m *ChatMessage) Type() Type {
	if m.msgType != "" {
		return m.msgType
	}
	return TypeChat
}

func (m *ChatMessage) Parts() Parts {
	return m.base.contentClone()
}

func (m *ChatMessage) Clone() Message {
	clone := *m
	clone.base = m.base.clone()
	return &clone
}

// FunctionMessage captures serialized tool/function responses.
type FunctionMessage struct {
	base messageBase
	Name string
}

func NewFunctionMessage(name string, text string, opts ...Option) *FunctionMessage {
	return &FunctionMessage{base: newMessageBase(partsFromText(text), opts...), Name: name}
}

func (m *FunctionMessage) Type() Type { return TypeFunction }

func (m *FunctionMessage) Parts() Parts {
	return m.base.contentClone()
}

func (m *FunctionMessage) Clone() Message {
	clone := *m
	clone.base = m.base.clone()
	return &clone
}

// ToolMessage stores tool output tied to a specific tool call.
type ToolMessage struct {
	base       messageBase
	ToolCallID string
}

func NewToolMessage(toolCallID string, text string, opts ...Option) *ToolMessage {
	return &ToolMessage{base: newMessageBase(partsFromText(text), opts...), ToolCallID: toolCallID}
}

func (m *ToolMessage) Type() Type { return TypeTool }

func (m *ToolMessage) Parts() Parts {
	return m.base.contentClone()
}

func (m *ToolMessage) Clone() Message {
	clone := *m
	clone.base = m.base.clone()
	return &clone
}

// BaseMessageChunk represents a streaming fragment of a message.
type BaseMessageChunk struct {
	msgType Type
	base    messageBase
}

func NewBaseMessageChunk(msgType Type, text string, opts ...Option) *BaseMessageChunk {
	return &BaseMessageChunk{msgType: msgType, base: newMessageBase(partsFromText(text), opts...)}
}

func (c *BaseMessageChunk) Type() Type {
	if c == nil {
		return ""
	}
	return c.msgType
}

func (c *BaseMessageChunk) Parts() Parts {
	return c.base.contentClone()
}

func (c *BaseMessageChunk) Clone() *BaseMessageChunk {
	clone := *c
	clone.base = c.base.clone()
	return &clone
}

func (c *BaseMessageChunk) Merge(other *BaseMessageChunk) (*BaseMessageChunk, error) {
	if other == nil {
		return c.Clone(), nil
	}
	if c.msgType != other.msgType {
		return nil, fmt.Errorf("message: cannot merge chunk types %q and %q", c.msgType, other.msgType)
	}
	merged := &BaseMessageChunk{msgType: c.msgType}
	merged.base = mergeBase(c.base, other.base)
	return merged, nil
}

// AIMessageChunk is a streaming fragment emitted by an AIMessage.
type AIMessageChunk struct {
	BaseMessageChunk
	ToolCalls []ToolCall
}

func NewAIMessageChunk(text string, opts ...Option) *AIMessageChunk {
	return &AIMessageChunk{BaseMessageChunk: *NewBaseMessageChunk(TypeAI, text, opts...)}
}

func (c *AIMessageChunk) Clone() *AIMessageChunk {
	clone := *c
	clone.BaseMessageChunk = *c.BaseMessageChunk.Clone()
	clone.ToolCalls = cloneToolCalls(c.ToolCalls)
	return &clone
}

func (c *AIMessageChunk) Merge(other *AIMessageChunk) (*AIMessageChunk, error) {
	if other == nil {
		return c.Clone(), nil
	}
	base, err := c.BaseMessageChunk.Merge(&other.BaseMessageChunk)
	if err != nil {
		return nil, err
	}
	merged := &AIMessageChunk{BaseMessageChunk: *base}
	merged.ToolCalls = append(cloneToolCalls(c.ToolCalls), cloneToolCalls(other.ToolCalls)...)
	return merged, nil
}

// NewTextPart is a helper to construct a TextPart.
func NewTextPart(text string) TextPart {
	return TextPart{Text: text}
}

func partsFromText(text string) Parts {
	return Parts{NewTextPart(text)}
}

func cloneParts(parts Parts) Parts {
	if len(parts) == 0 {
		return nil
	}
	out := make(Parts, len(parts))
	for i, part := range parts {
		if part == nil {
			out[i] = nil
			continue
		}
		out[i] = clonePart(part)
	}
	return out
}

//nolint:gocyclo // Part cloning requires handling many part types
func clonePart(part Part) Part {
	switch v := part.(type) {
	case TextPart:
		return TextPart{Text: v.Text}
	case *TextPart:
		if v == nil {
			return nil
		}
		clone := *v
		return &clone
	case DataPart:
		return DataPart{Data: cloneMap(v.Data)}
	case *DataPart:
		if v == nil {
			return nil
		}
		clone := *v
		clone.Data = cloneMap(v.Data)
		return &clone
	case FilePart:
		return FilePart{File: cloneFilePartContent(v.File), MimeType: v.MimeType, Name: v.Name}
	case *FilePart:
		if v == nil {
			return nil
		}
		clone := *v
		clone.File = cloneFilePartContent(v.File)
		return &clone
	case FunctionCallPart:
		return FunctionCallPart{FunctionCall: cloneFunctionCall(v.FunctionCall)}
	case *FunctionCallPart:
		if v == nil {
			return nil
		}
		clone := *v
		clone.FunctionCall = cloneFunctionCall(v.FunctionCall)
		return &clone
	case FunctionResponsePart:
		return FunctionResponsePart{FunctionResponse: cloneFunctionResponse(v.FunctionResponse)}
	case *FunctionResponsePart:
		if v == nil {
			return nil
		}
		clone := *v
		clone.FunctionResponse = cloneFunctionResponse(v.FunctionResponse)
		return &clone
	default:
		return v
	}
}

func cloneFunctionCall(call *FunctionCall) *FunctionCall {
	if call == nil {
		return nil
	}
	clone := *call
	return &clone
}

func cloneFunctionResponse(resp *FunctionResponse) *FunctionResponse {
	if resp == nil {
		return nil
	}
	clone := *resp
	return &clone
}

func cloneFilePartContent(content FilePartContent) FilePartContent {
	switch v := content.(type) {
	case nil:
		return nil
	case FileRawBytes:
		return FileRawBytes{Bytes: cloneBytes(v.Bytes)}
	case *FileRawBytes:
		if v == nil {
			return nil
		}
		return &FileRawBytes{Bytes: cloneBytes(v.Bytes)}
	case FileBase64:
		return FileBase64{Base64: v.Base64}
	case *FileBase64:
		if v == nil {
			return nil
		}
		clone := *v
		return &clone
	case FilePath:
		return FilePath{Path: v.Path}
	case *FilePath:
		if v == nil {
			return nil
		}
		clone := *v
		return &clone
	case FileURI:
		return FileURI{URI: v.URI}
	case *FileURI:
		if v == nil {
			return nil
		}
		clone := *v
		return &clone
	default:
		return v
	}
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	clone := make([]byte, len(src))
	copy(clone, src)
	return clone
}

func cloneMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]any, len(source))
	for k, v := range source {
		out[k] = v
	}
	return out
}

func cloneToolCalls(calls []ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, len(calls))
	for i, call := range calls {
		out[i] = ToolCall{ID: call.ID, Name: call.Name, Type: call.Type}
		if len(call.Arguments) > 0 {
			out[i].Arguments = cloneMap(call.Arguments)
		}
	}
	return out
}

func mergeBase(a, b messageBase) messageBase {
	merged := messageBase{}
	merged.content = append(cloneParts(a.content), cloneParts(b.content)...)
	return merged
}
