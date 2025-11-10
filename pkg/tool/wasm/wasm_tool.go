package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Compile-time check to ensure WASMTool implements tool.Tool interface
var _ tool.Tool = (*WASMTool)(nil)

// ToolSchema describes a WASM tool's interface for LLM function calling.
type ToolSchema struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Parameters  *ParameterSchema `json:"parameters"`
}

// ParameterSchema describes the tool's input parameters.
type ParameterSchema struct {
	Type       string                    `json:"type"` // Always "object"
	Properties map[string]PropertySchema `json:"properties"`
	Required   []string                  `json:"required,omitempty"`
}

// PropertySchema describes a single parameter.
type PropertySchema struct {
	Type        string          `json:"type"` // "string", "number", "boolean", "array", "object"
	Description string          `json:"description,omitempty"`
	Enum        []string        `json:"enum,omitempty"`
	Items       *PropertySchema `json:"items,omitempty"` // For arrays
}

// WASMTool executes WebAssembly modules with strict sandboxing.
// It provides kernel-level isolation that cannot be bypassed by tool code.
type WASMTool struct {
	name        string
	description string
	wasmBytes   []byte
	policy      *SandboxPolicy
	schema      *ToolSchema

	// Runtime state
	runtime     wazero.Runtime
	compiled    wazero.CompiledModule
	runtimeOpts *RuntimeOptions
}

// RuntimeOptions configures the WASM runtime behavior.
type RuntimeOptions struct {
	// CompilationCache caches compiled WASM modules for faster startup
	CompilationCache wazero.CompilationCache

	// StdoutWriter redirects stdout output (nil = discard)
	StdoutWriter io.Writer

	// StderrWriter redirects stderr output (nil = discard)
	StderrWriter io.Writer

	// StdinReader provides stdin input (nil = empty)
	StdinReader io.Reader
}

// NewWASMTool creates a new WASM tool from compiled bytecode.
// The tool is immediately validated and compiled, but not instantiated until Call().
func NewWASMTool(ctx context.Context, name, description string, wasmBytes []byte, opts ...WASMToolOption) (*WASMTool, error) {
	if name == "" {
		return nil, errors.New("tool name cannot be empty")
	}
	if len(wasmBytes) == 0 {
		return nil, errors.New("WASM bytecode cannot be empty")
	}

	tool := &WASMTool{
		name:        name,
		description: description,
		wasmBytes:   wasmBytes,
		policy:      DefaultSandboxPolicy(),
		runtimeOpts: &RuntimeOptions{},
	}

	// Apply options
	for _, opt := range opts {
		opt(tool)
	}

	// Create runtime with compilation cache if provided
	runtimeConfig := wazero.NewRuntimeConfig()
	if tool.runtimeOpts.CompilationCache != nil {
		runtimeConfig = runtimeConfig.WithCompilationCache(tool.runtimeOpts.CompilationCache)
	}

	tool.runtime = wazero.NewRuntimeWithConfig(ctx, runtimeConfig)

	// Compile module (validates WASM)
	compiled, err := tool.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		tool.runtime.Close(ctx)
		return nil, fmt.Errorf("failed to compile WASM module: %w", err)
	}
	tool.compiled = compiled

	return tool, nil
}

// WASMToolOption configures a WASMTool.
type WASMToolOption func(*WASMTool)

// WithPolicy sets the sandbox policy for the tool.
func WithPolicy(policy *SandboxPolicy) WASMToolOption {
	return func(t *WASMTool) {
		t.policy = policy
	}
}

// WithCompilationCache enables compilation caching for faster instantiation.
func WithCompilationCache(cache wazero.CompilationCache) WASMToolOption {
	return func(t *WASMTool) {
		t.runtimeOpts.CompilationCache = cache
	}
}

// WithStdout redirects stdout to the provided writer.
func WithStdout(w io.Writer) WASMToolOption {
	return func(t *WASMTool) {
		t.runtimeOpts.StdoutWriter = w
		t.policy.AllowStdout = true
	}
}

// WithStderr redirects stderr to the provided writer.
func WithStderr(w io.Writer) WASMToolOption {
	return func(t *WASMTool) {
		t.runtimeOpts.StderrWriter = w
		t.policy.AllowStderr = true
	}
}

// WithSchema sets the tool schema for LLM integration.
func WithSchema(schema *ToolSchema) WASMToolOption {
	return func(t *WASMTool) {
		t.schema = schema
	}
}

// Name returns the tool name.
func (w *WASMTool) Name() string {
	return w.name
}

// Description returns the tool description.
func (w *WASMTool) Description() string {
	return w.description
}

// Definition returns the tool definition for LLM function calling.
// Implements the tool.Tool interface.
func (w *WASMTool) Definition() *tool.Definition {
	var params map[string]any

	if w.schema == nil {
		// Fallback: minimal definition without parameters
		params = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	} else {
		params = map[string]any{
			"type":       w.schema.Parameters.Type,
			"properties": w.schema.Parameters.Properties,
		}
		if len(w.schema.Parameters.Required) > 0 {
			params["required"] = w.schema.Parameters.Required
		}
	}

	name := w.name
	description := w.description
	if w.schema != nil {
		name = w.schema.Name
		description = w.schema.Description
	}

	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        name,
			Description: description,
			Parameters:  params,
		},
	}
}

// Call executes the WASM tool with JSON arguments.
// The tool is instantiated, executed, and immediately destroyed for isolation.
func (w *WASMTool) Call(ctx context.Context, argsJSON string) (any, error) {
	// Apply timeout from policy
	if w.policy.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.policy.Timeout)
		defer cancel()
	}

	// Instantiate module with policy-based configuration
	mod, err := w.instantiateWithPolicy(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate WASM module: %w", err)
	}
	defer mod.Close(ctx)

	// Get the execute function
	executeFunc := mod.ExportedFunction("execute")
	if executeFunc == nil {
		return nil, errors.New("WASM module does not export 'execute' function")
	}

	// Allocate memory for input JSON
	inputPtr, inputLen, err := w.allocateString(ctx, mod, argsJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate input memory: %w", err)
	}

	// Call execute(inputPtr, inputLen) - stores result in globals
	_, err = executeFunc.Call(ctx, uint64(inputPtr), uint64(inputLen))
	if err != nil {
		return nil, fmt.Errorf("WASM execution failed: %w", err)
	}

	// Get result pointer and length from exported functions
	getPtrFunc := mod.ExportedFunction("get_result_ptr")
	getLenFunc := mod.ExportedFunction("get_result_len")
	if getPtrFunc == nil || getLenFunc == nil {
		return nil, errors.New("WASM module does not export get_result_ptr/get_result_len functions")
	}

	ptrResults, err := getPtrFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get result pointer: %w", err)
	}

	lenResults, err := getLenFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get result length: %w", err)
	}

	resultPtr := uint32(ptrResults[0])
	resultLen := uint32(lenResults[0])

	if resultLen == 0 {
		return nil, fmt.Errorf("WASM returned empty result")
	}

	resultJSON, err := w.readString(mod, resultPtr, resultLen)
	if err != nil {
		return nil, fmt.Errorf("failed to read result: %w", err)
	}

	// Parse result JSON
	var result any
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return nil, fmt.Errorf("failed to parse result JSON: %w", err)
	}

	return result, nil
}

// instantiateWithPolicy creates a module instance with sandbox policy applied.
func (w *WASMTool) instantiateWithPolicy(ctx context.Context) (api.Module, error) {
	// Configure WASI with restrictions
	config := wazero.NewModuleConfig().WithName(w.name)

	// Configure I/O based on policy
	if w.policy.AllowStdout && w.runtimeOpts.StdoutWriter != nil {
		config = config.WithStdout(w.limitWriter(w.runtimeOpts.StdoutWriter))
	} else {
		config = config.WithStdout(io.Discard)
	}

	if w.policy.AllowStderr && w.runtimeOpts.StderrWriter != nil {
		config = config.WithStderr(w.limitWriter(w.runtimeOpts.StderrWriter))
	} else {
		config = config.WithStderr(io.Discard)
	}

	if w.policy.AllowStdin && w.runtimeOpts.StdinReader != nil {
		config = config.WithStdin(w.runtimeOpts.StdinReader)
	}

	// Configure environment based on policy
	// Note: Cannot completely disable environment in Wazero, but we can set to minimal
	if !w.policy.AllowEnvironment {
		// Minimal environment - no custom variables
	}

	// Configure filesystem based on policy
	if !w.policy.AllowFilesystem {
		config = config.WithFS(nil) // No filesystem access
	}

	// Configure time based on policy
	if w.policy.Deterministic && w.policy.FixedTimestamp != nil {
		// TODO: Implement deterministic time via custom WASI implementation
	}

	// Instantiate WASI (only if not already instantiated)
	if w.runtime.Module("wasi_snapshot_preview1") == nil {
		wasi_snapshot_preview1.MustInstantiate(ctx, w.runtime)
	}

	// Instantiate the module
	mod, err := w.runtime.InstantiateModule(ctx, w.compiled, config)
	if err != nil {
		return nil, err
	}

	return mod, nil
}

// limitWriter wraps a writer to enforce MaxOutputSize.
func (w *WASMTool) limitWriter(wr io.Writer) io.Writer {
	if w.policy.MaxOutputSize > 0 {
		return &limitedWriter{
			Writer: wr,
			Limit:  w.policy.MaxOutputSize,
		}
	}
	return wr
}

// limitedWriter implements io.Writer with a size limit.
type limitedWriter struct {
	Writer  io.Writer
	Limit   int64
	Written int64
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	if lw.Written >= lw.Limit {
		return 0, fmt.Errorf("output size limit exceeded: %d bytes", lw.Limit)
	}

	remaining := lw.Limit - lw.Written
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}

	n, err = lw.Writer.Write(p)
	lw.Written += int64(n)
	return n, err
}

// Close releases resources associated with the WASM tool.
func (w *WASMTool) Close(ctx context.Context) error {
	if w.runtime != nil {
		return w.runtime.Close(ctx)
	}
	return nil
}
