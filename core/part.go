package core

import "maps"

// Part is a polymorphic segment of role-based content. Concrete part types
// implement the unexported isPart marker to form a closed set.
type Part interface{ isPart() }

// Parts is a slice of core.Part.
type Parts = []Part

// Role indicates the conversational role associated with a Content message.
// It is a string-based type for seamless JSON serialization.
type Role string

const (
	// RoleUser indicates the user role in a conversation.
	RoleUser Role = "user"

	// RoleAssistant indicates the assistant role in a conversation.
	RoleAssistant Role = "assistant"

	// RoleTool indicates the tool role in a conversation.
	RoleTool Role = "tool"

	// RoleSystem indicates the system role in a conversation.
	RoleSystem Role = "system"
)

// TextPart is a plain UTF-8 text content segment.
type TextPart struct {
	Text string // Plain UTF-8 text
}

// isPart implements the Part interface for TextPart.
func (TextPart) isPart() {}

// DataPart is a structured data segment (for example a JSON object map).
type DataPart struct {
	Data map[string]any // Structured key/value payload
}

// isPart implements the Part interface for DataPart.
func (DataPart) isPart() {}

// FilePart represents a file segment within a message or artifact. The file content can be
// provided as a local file path, raw bytes, base64-encoded bytes, or a URL.
type FilePart struct {
	// The file content, represented as one of: FilePath | FileRawBytes | FileBase64 | FileURI.
	File FilePartContent

	// An optional MIME type of the file (e.g., "application/pdf").
	MimeType string

	// An optional name for the file (e.g., "document.pdf").
	Name string
}

// isPart implements the Part interface for FilePart.
func (FilePart) isPart() {}

// FilePartContent is a discriminated union representing content of a FilePart.
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

// FileURI represents a file with its content located at a specific URI.
type FileURI struct {
	URI string
}

func (FileRawBytes) isFilePartContent() {}
func (FileBase64) isFilePartContent()   {}
func (FilePath) isFilePartContent()     {}
func (FileURI) isFilePartContent()      {}

// FunctionCall describes a tool/function invocation request.
type FunctionCall struct {
	ID        string `json:"id,omitempty"`        // Optional stable id (can be supplied later)
	Name      string `json:"name"`                // Tool / function name
	Arguments string `json:"arguments,omitempty"` // Serialized argument payload (e.g. JSON)
}

// FunctionCallPart wraps a FunctionCall as a content part.
type FunctionCallPart struct {
	FunctionCall *FunctionCall
}

// isPart implements the Part interface for FunctionCallPart.
func (FunctionCallPart) isPart() {}

// FunctionResponse describes the outcome of a function call.
type FunctionResponse struct {
	ID       string `json:"id,omitempty"`       // Matches originating FunctionCall ID
	Name     string `json:"name"`               // Function name
	Response any    `json:"response,omitempty"` // Successful result (any shape)
}

// FunctionResponsePart wraps a FunctionResponse as a content part.
type FunctionResponsePart struct {
	FunctionResponse *FunctionResponse
}

// isPart implements the Part interface for FunctionResponsePart.
func (FunctionResponsePart) isPart() {}

// NewPartFromText creates a new TextPart from the given text.
func NewPartFromText(text string) Part {
	return &TextPart{
		Text: text,
	}
}

// NewPartFromFileRawBytes creates a FilePart from raw bytes.
func NewPartFromFileRawBytes(name string, bytes []byte) Part {
	// Defensive copy of bytes to avoid external mutation
	b := append([]byte(nil), bytes...)
	return &FilePart{
		Name: name,
		File: &FileRawBytes{Bytes: b},
	}
}

// NewPartFromFileBase64 creates a FilePart from a base64-encoded string.
func NewPartFromFileBase64(name, base64 string) Part {
	return &FilePart{
		Name: name,
		File: &FileBase64{Base64: base64},
	}
}

// NewPartFromFilePath creates a FilePart from a local filesystem path.
func NewPartFromFilePath(name, path string) Part {
	return &FilePart{
		Name: name,
		File: &FilePath{Path: path},
	}
}

// NewPartFromFileURI creates a FilePart from a URI.
func NewPartFromFileURI(name, uri string) Part {
	return &FilePart{
		Name: name,
		File: &FileURI{URI: uri},
	}
}

// NewPartFromFileBytes is deprecated: it created a FilePart from a base64 string.
// Use NewPartFromFileBase64 instead.
func NewPartFromFileBytes(name, bytes string) Part {
	return NewPartFromFileBase64(name, bytes)
}

// NewPartFromFunctionCall creates a new FunctionCallPart from the given name and arguments.
func NewPartFromFunctionCall(id, name, args string) Part {
	return &FunctionCallPart{
		FunctionCall: &FunctionCall{
			ID:        id,
			Name:      name,
			Arguments: args,
		},
	}
}

// NewPartFromFunctionResponse creates a new FunctionResponsePart from the given name and response.
func NewPartFromFunctionResponse(id, name string, response any) Part {
	fr := &FunctionResponse{ID: id, Name: name, Response: response}

	return &FunctionResponsePart{
		FunctionResponse: fr,
	}
}

// clonePart deep clones a Part based on its concrete type.
func clonePart(p Part) Part {
	if p == nil {
		return nil
	}

	switch pt := p.(type) {
	case *TextPart:
		return &TextPart{Text: pt.Text}

	case *DataPart:
		var data map[string]any
		if pt.Data != nil {
			data = maps.Clone(pt.Data)
		}
		return &DataPart{Data: data}

	case *FilePart:
		// Deep-copy the discriminated union
		var fileCopy FilePartContent
		switch f := pt.File.(type) {
		case *FileRawBytes:
			b := append([]byte(nil), f.Bytes...)
			fileCopy = &FileRawBytes{Bytes: b}

		case *FileBase64:
			fileCopy = &FileBase64{Base64: f.Base64}

		case *FilePath:
			fileCopy = &FilePath{Path: f.Path}

		case *FileURI:
			fileCopy = &FileURI{URI: f.URI}

		default:
			// Unknown implementation; fall back to shallow copy
			fileCopy = f
		}

		return &FilePart{
			File:     fileCopy,
			MimeType: pt.MimeType,
			Name:     pt.Name,
		}

	case *FunctionCallPart:
		var fc *FunctionCall
		if pt.FunctionCall != nil {
			// Deep copy the pointed struct
			fc = &FunctionCall{ID: pt.FunctionCall.ID, Name: pt.FunctionCall.Name, Arguments: pt.FunctionCall.Arguments}
		}
		return &FunctionCallPart{FunctionCall: fc}

	case *FunctionResponsePart:
		var fr *FunctionResponse
		if pt.FunctionResponse != nil {
			fr = &FunctionResponse{
				ID:       pt.FunctionResponse.ID,
				Name:     pt.FunctionResponse.Name,
				Response: pt.FunctionResponse.Response,
			}
		}
		return &FunctionResponsePart{FunctionResponse: fr}

	default:
		// unknown Part type: shallow copy
		return pt
	}
}

// ClonePart creates a deep copy of a single Part. Exported for use by other packages
// that need to defensively copy Parts without depending on Content.Clone.
func ClonePart(p Part) Part {
	return clonePart(p)
}
