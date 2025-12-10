package message

import (
	"encoding/json"
	"fmt"
)

// SerializableMessage wraps a Message for JSON serialization.
// This enables message.Message interfaces to be marshaled/unmarshaled through Redis.
type SerializableMessage struct {
	MsgType    Type         `json:"type"`
	Parts      []SerialPart `json:"parts"`
	ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`
	Name       string       `json:"name,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

// PartKind represents the type of a serializable content part.
type PartKind string

// Part kind constants for serialization.
const (
	PartKindText             PartKind = "text"
	PartKindData             PartKind = "data"
	PartKindFile             PartKind = "file"
	PartKindFunctionCall     PartKind = "function_call"
	PartKindFunctionResponse PartKind = "function_response"
)

// File type constants for serialization.
const (
	fileTypeRaw    = "raw"
	fileTypeBase64 = "base64"
	fileTypePath   = "path"
	fileTypeURI    = "uri"
)

// SerialPart represents a serializable content part.
type SerialPart struct {
	Kind PartKind       `json:"kind"`
	Data map[string]any `json:"data"`
}

// ToSerializable converts a Message interface to a serializable form.
//
//nolint:gocyclo // Message type conversion requires exhaustive type checking
func ToSerializable(msg Message) *SerializableMessage {
	if msg == nil {
		return nil
	}

	sm := &SerializableMessage{
		MsgType: msg.Type(),
		Parts:   make([]SerialPart, 0, len(msg.Parts())),
	}

	// Convert parts to serializable form
	for _, part := range msg.Parts() {
		sp := SerialPart{Data: make(map[string]any)}

		switch p := part.(type) {
		case TextPart:
			sp.Kind = PartKindText
			sp.Data["text"] = p.Text
		case *TextPart:
			if p != nil {
				sp.Kind = PartKindText
				sp.Data["text"] = p.Text
			}
		case DataPart:
			sp.Kind = PartKindData
			sp.Data["data"] = p.Data
		case *DataPart:
			if p != nil {
				sp.Kind = PartKindData
				sp.Data["data"] = p.Data
			}
		case FunctionCallPart:
			sp.Kind = PartKindFunctionCall
			if p.FunctionCall != nil {
				sp.Data["id"] = p.FunctionCall.ID
				sp.Data["name"] = p.FunctionCall.Name
				sp.Data["arguments"] = p.FunctionCall.Arguments
			}
		case *FunctionCallPart:
			if p != nil && p.FunctionCall != nil {
				sp.Kind = PartKindFunctionCall
				sp.Data["id"] = p.FunctionCall.ID
				sp.Data["name"] = p.FunctionCall.Name
				sp.Data["arguments"] = p.FunctionCall.Arguments
			}
		case FunctionResponsePart:
			sp.Kind = PartKindFunctionResponse
			if p.FunctionResponse != nil {
				sp.Data["id"] = p.FunctionResponse.ID
				sp.Data["name"] = p.FunctionResponse.Name
				sp.Data["response"] = p.FunctionResponse.Response
			}
		case *FunctionResponsePart:
			if p != nil && p.FunctionResponse != nil {
				sp.Kind = PartKindFunctionResponse
				sp.Data["id"] = p.FunctionResponse.ID
				sp.Data["name"] = p.FunctionResponse.Name
				sp.Data["response"] = p.FunctionResponse.Response
			}
		case FilePart:
			sp.Kind = PartKindFile
			sp.Data["mime_type"] = p.MimeType
			sp.Data["name"] = p.Name
			// Serialize file content based on type
			switch fc := p.File.(type) {
			case FileRawBytes:
				sp.Data["file_type"] = fileTypeRaw
				sp.Data["bytes"] = fc.Bytes
			case FileBase64:
				sp.Data["file_type"] = fileTypeBase64
				sp.Data["base64"] = fc.Base64
			case FilePath:
				sp.Data["file_type"] = fileTypePath
				sp.Data["path"] = fc.Path
			case FileURI:
				sp.Data["file_type"] = fileTypeURI
				sp.Data["uri"] = fc.URI
			}
		case *FilePart:
			if p != nil {
				sp.Kind = PartKindFile
				sp.Data["mime_type"] = p.MimeType
				sp.Data["name"] = p.Name
				switch fc := p.File.(type) {
				case FileRawBytes:
					sp.Data["file_type"] = fileTypeRaw
					sp.Data["bytes"] = fc.Bytes
				case FileBase64:
					sp.Data["file_type"] = fileTypeBase64
					sp.Data["base64"] = fc.Base64
				case FilePath:
					sp.Data["file_type"] = fileTypePath
					sp.Data["path"] = fc.Path
				case FileURI:
					sp.Data["file_type"] = fileTypeURI
					sp.Data["uri"] = fc.URI
				}
			}
		default:
			// Unknown part type, skip
			continue
		}

		sm.Parts = append(sm.Parts, sp)
	}

	// Add type-specific fields
	switch m := msg.(type) {
	case *AIMessage:
		sm.ToolCalls = m.ToolCalls
		sm.Name = m.Name
	case *FunctionMessage:
		sm.Name = m.Name
	case *ToolMessage:
		sm.ToolCallID = m.ToolCallID
	}

	return sm
}

// FromSerializable converts a SerializableMessage back to a Message interface.
//
//nolint:gocyclo // Message deserialization requires exhaustive type reconstruction
func FromSerializable(sm *SerializableMessage) (Message, error) {
	if sm == nil {
		return nil, nil
	}

	// Convert serialized parts back to Parts
	parts := make(Parts, 0, len(sm.Parts))
	for _, sp := range sm.Parts {
		switch sp.Kind {
		case PartKindText:
			if text, ok := sp.Data["text"].(string); ok {
				parts = append(parts, TextPart{Text: text})
			}
		case PartKindData:
			if data, ok := sp.Data["data"].(map[string]any); ok {
				parts = append(parts, DataPart{Data: data})
			}
		case PartKindFunctionCall:
			fc := &FunctionCall{}
			if id, ok := sp.Data["id"].(string); ok {
				fc.ID = id
			}
			if name, ok := sp.Data["name"].(string); ok {
				fc.Name = name
			}
			if args, ok := sp.Data["arguments"].(string); ok {
				fc.Arguments = args
			}
			parts = append(parts, FunctionCallPart{FunctionCall: fc})
		case PartKindFunctionResponse:
			fr := &FunctionResponse{}
			if id, ok := sp.Data["id"].(string); ok {
				fr.ID = id
			}
			if name, ok := sp.Data["name"].(string); ok {
				fr.Name = name
			}
			if resp, ok := sp.Data["response"]; ok {
				fr.Response = resp
			}
			parts = append(parts, FunctionResponsePart{FunctionResponse: fr})
		case PartKindFile:
			fp := FilePart{}
			if mime, ok := sp.Data["mime_type"].(string); ok {
				fp.MimeType = mime
			}
			if name, ok := sp.Data["name"].(string); ok {
				fp.Name = name
			}
			// Deserialize file content
			//nolint:nestif // File content deserialization requires nested type checking
			if fileType, ok := sp.Data["file_type"].(string); ok {
				switch fileType {
				case fileTypeRaw:
					if bytes, ok := sp.Data["bytes"].([]byte); ok {
						fp.File = FileRawBytes{Bytes: bytes}
					}
				case fileTypeBase64:
					if b64, ok := sp.Data["base64"].(string); ok {
						fp.File = FileBase64{Base64: b64}
					}
				case fileTypePath:
					if path, ok := sp.Data["path"].(string); ok {
						fp.File = FilePath{Path: path}
					}
				case fileTypeURI:
					if uri, ok := sp.Data["uri"].(string); ok {
						fp.File = FileURI{URI: uri}
					}
				}
			}
			parts = append(parts, fp)
		}
	}

	// Create appropriate message type
	switch sm.MsgType {
	case TypeSystem:
		return NewSystemMessage(parts), nil
	case TypeHuman:
		return NewHumanMessage(parts), nil
	case TypeAI:
		msg := NewAIMessage(parts)
		msg.ToolCalls = sm.ToolCalls
		msg.Name = sm.Name
		return msg, nil
	case TypeChat:
		return NewChatMessage(string(sm.MsgType), NewSystemMessage(parts).String()), nil
	case TypeFunction:
		if len(parts) > 0 {
			return NewFunctionMessage(sm.Name, NewSystemMessage(parts).String()), nil
		}
		return NewFunctionMessage(sm.Name, ""), nil
	case TypeTool:
		if len(parts) > 0 {
			return NewToolMessage(sm.ToolCallID, NewSystemMessage(parts).String()), nil
		}
		return NewToolMessage(sm.ToolCallID, ""), nil
	default:
		return nil, fmt.Errorf("unknown message type: %s", sm.MsgType)
	}
}

// MarshalMessage implements json.Marshaler for Message interface.
func MarshalMessage(msg Message) ([]byte, error) {
	sm := ToSerializable(msg)
	return json.Marshal(sm)
}

// UnmarshalMessage implements json.Unmarshaler for Message interface.
func UnmarshalMessage(data []byte) (Message, error) {
	var sm SerializableMessage
	if err := json.Unmarshal(data, &sm); err != nil {
		return nil, err
	}
	return FromSerializable(&sm)
}
