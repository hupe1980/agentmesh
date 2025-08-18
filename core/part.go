package core

// Part represents a polymorphic segment of role-based content. Concrete part
// types implement the unexported isPart marker enabling a closed set.
type Part interface{ isPart() }

// TextPart is a plain text content segment.
type TextPart struct {
	Text     string         // Plain UTF-8 text
	Metadata map[string]any // Optional producer-provided metadata
}

// isPart implements the Part interface for TextPart.
func (TextPart) isPart() {}

// DataPart is a structured data segment (e.g., JSON object map).
type DataPart struct {
	Data     map[string]any // Structured key/value payload
	Metadata map[string]any
}

// isPart implements the Part interface for DataPart.
func (DataPart) isPart() {}

// FilePart is a file attachment segment.
type FilePart struct {
	File     FilePartFile // File metadata / reference
	Metadata map[string]any
}

// isPart implements the Part interface for FilePart.
func (FilePart) isPart() {}

// FunctionCall describes a tool/function invocation request.
type FunctionCall struct {
	ID        string `json:"id,omitempty"`        // Optional stable id (can be supplied later)
	Name      string `json:"name"`                // Tool / function name
	Arguments string `json:"arguments,omitempty"` // Serialized argument payload (e.g. JSON)
}

// FunctionCallPart wraps a FunctionCall as a content part.
type FunctionCallPart struct {
	FunctionCall FunctionCall
	Metadata     map[string]any
}

// isPart implements the Part interface for FunctionCallPart.
func (FunctionCallPart) isPart() {}

// FunctionResponse describes the outcome of a function call.
type FunctionResponse struct {
	ID       string      `json:"id,omitempty"`       // Matches originating FunctionCall ID
	Name     string      `json:"name"`               // Function name
	Response interface{} `json:"response,omitempty"` // Successful result (any shape)
	Error    string      `json:"error,omitempty"`    // Populated on failure
}

// FunctionResponsePart wraps a FunctionResponse as a content part.
type FunctionResponsePart struct {
	FunctionResponse FunctionResponse
	Metadata         map[string]any
}

// isPart implements the Part interface for FunctionResponsePart.
func (FunctionResponsePart) isPart() {}

// FilePartFile represents a file attachment segment.
type FilePartFile struct {
	Bytes    string  // Base64 encoded contents (if inlined)
	MimeType *string // Optional MIME type
	Name     *string // Original filename hint
	URI      string  // External retrieval URI (if not inlined)
}

// FileWithBytes represents a file provided with inlined (base64 encoded) bytes
// and optional MIME type / original name metadata. Used when constructing
// FilePartFile values programmatically.
type FileWithBytes struct {
	Bytes    string
	MimeType *string
	Name     *string
}

// FileWithUri represents a file available at an external URI with optional
// MIME type / original name metadata.
type FileWithUri struct {
	MimeType *string
	Name     *string
	URI      string
}

// Content holds role + ordered parts.
type Content struct {
	Role  string `json:"role,omitempty"` // Conversation role (user, assistant, tool, system,...)
	Parts []Part `json:"parts"`          // Ordered heterogeneous parts
}
