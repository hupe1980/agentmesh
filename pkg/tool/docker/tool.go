package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hupe1980/agentmesh/internal/jsonschema"
	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// Compile-time check to ensure Tool implements tool.Tool interface.
var _ tool.Tool = (*Tool)(nil)

// Args defines the JSON schema for the Docker tool input.
type Args struct {
	Command string `json:"command" jsonschema:"required,description=The command to execute in the container"`
}

// Options configures a Docker tool.
type Options struct {
	// Description explains what the tool does (shown to LLM)
	Description string

	// Timeout for command execution (default: 30s)
	Timeout time.Duration

	// NetworkMode controls network access ("none", "bridge", "host")
	// Default is "none" for maximum isolation.
	NetworkMode string

	// AllowedCommands restricts which commands can be executed (optional).
	// If set, only commands starting with one of these prefixes are allowed.
	AllowedCommands []string

	// MemoryLimit in bytes (default: 256MB)
	MemoryLimit int64

	// CPUQuota (100000 = 1 CPU, default: 50000 = 0.5 CPU)
	CPUQuota int64

	// PullImage pulls the image before first use
	PullImage bool

	// User sets the user inside the container (e.g., "1000:1000")
	User string

	// WorkingDir sets the working directory inside the container
	WorkingDir string
}

// Option is a functional option for configuring a Docker tool.
type Option func(*Options)

// WithDescription sets the tool description shown to the LLM.
func WithDescription(description string) Option {
	return func(o *Options) {
		o.Description = description
	}
}

// WithTimeout sets the execution timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		o.Timeout = timeout
	}
}

// WithNetworkMode sets the container network mode.
// Valid values: "none" (default), "bridge", "host".
func WithNetworkMode(mode string) Option {
	return func(o *Options) {
		o.NetworkMode = mode
	}
}

// WithAllowedCommands restricts which commands can be executed.
// Only commands starting with one of these prefixes are allowed.
func WithAllowedCommands(commands ...string) Option {
	return func(o *Options) {
		o.AllowedCommands = commands
	}
}

// WithMemoryLimit sets the container memory limit in bytes.
func WithMemoryLimit(limit int64) Option {
	return func(o *Options) {
		o.MemoryLimit = limit
	}
}

// WithCPUQuota sets the CPU quota (100000 = 1 CPU).
func WithCPUQuota(quota int64) Option {
	return func(o *Options) {
		o.CPUQuota = quota
	}
}

// WithPullImage enables pulling the image before running.
func WithPullImage(pull bool) Option {
	return func(o *Options) {
		o.PullImage = pull
	}
}

// WithUser sets the user inside the container (e.g., "1000:1000").
func WithUser(user string) Option {
	return func(o *Options) {
		o.User = user
	}
}

// WithWorkingDir sets the working directory inside the container.
func WithWorkingDir(dir string) Option {
	return func(o *Options) {
		o.WorkingDir = dir
	}
}

// Tool implements tool.Tool for Docker-based command execution.
// It provides sandboxed execution of containerized commands with
// configurable resource limits and network isolation.
type Tool struct {
	name   string
	image  string
	opts   Options
	runner *Runner
	schema map[string]any
}

// NewTool creates a new Docker tool.
//
// Parameters:
//   - name: Tool name exposed to the LLM (snake_case recommended)
//   - image: Docker image to use (e.g., "python:3.12-slim")
//   - optFns: Functional options for configuration
//
// Example:
//
//	tool, err := docker.NewTool("python_exec", "python:3.12-slim",
//	    docker.WithDescription("Execute Python code"),
//	    docker.WithTimeout(30*time.Second),
//	    docker.WithNetworkMode("none"),
//	)
func NewTool(name, image string, optFns ...Option) (*Tool, error) {
	if err := validate.All(
		validate.NotEmpty(name, "tool name"),
		validate.NotEmpty(image, "docker image"),
	); err != nil {
		return nil, err
	}

	opts := Options{
		Timeout:     30 * time.Second,
		NetworkMode: "none",            // Isolated by default
		MemoryLimit: 256 * 1024 * 1024, // 256MB
		CPUQuota:    50000,             // 0.5 CPU
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	runner, err := NewRunner()
	if err != nil {
		return nil, err
	}

	schema, err := jsonschema.MapFromStruct(Args{})
	if err != nil {
		_ = runner.Close()
		return nil, fmt.Errorf("docker/tool: failed to create schema: %w", err)
	}

	return &Tool{
		name:   name,
		image:  image,
		opts:   opts,
		runner: runner,
		schema: schema,
	}, nil
}

// Name returns the tool name.
func (t *Tool) Name() string { return t.name }

// Description returns the tool description.
func (t *Tool) Description() string { return t.opts.Description }

// Definition returns the tool definition with JSON schema.
func (t *Tool) Definition() *tool.Definition {
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        t.name,
			Description: t.opts.Description,
			Parameters:  t.schema,
		},
	}
}

// Call executes the tool with JSON-serialized arguments.
// The command is parsed from the JSON input and executed in a Docker container.
func (t *Tool) Call(ctx context.Context, argsJSON string) (any, error) {
	var args Args
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("docker/tool: invalid arguments: %w", err)
	}

	// Validate command if restrictions are set
	if len(t.opts.AllowedCommands) > 0 {
		if !t.isCommandAllowed(args.Command) {
			return nil, fmt.Errorf("docker/tool: command not allowed: %s", args.Command)
		}
	}

	// Parse command into args
	cmdParts := strings.Fields(args.Command)
	if len(cmdParts) == 0 {
		return nil, fmt.Errorf("docker/tool: empty command")
	}

	result, err := t.runner.Run(ctx, Config{
		Image:       t.image,
		Command:     cmdParts,
		Timeout:     t.opts.Timeout,
		NetworkMode: t.opts.NetworkMode,
		AutoRemove:  true,
		MemoryLimit: t.opts.MemoryLimit,
		CPUQuota:    t.opts.CPUQuota,
		PullImage:   t.opts.PullImage,
		User:        t.opts.User,
		WorkingDir:  t.opts.WorkingDir,
	})
	if err != nil {
		return nil, err
	}

	// Format output for LLM
	output := string(result.Stdout)
	if len(result.Stderr) > 0 {
		output += "\n[stderr]: " + string(result.Stderr)
	}

	if result.ExitCode != 0 {
		output += fmt.Sprintf("\n[exit code]: %d", result.ExitCode)
	}

	return output, nil
}

// isCommandAllowed checks if the command is in the allowed list.
func (t *Tool) isCommandAllowed(cmd string) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}

	cmdBase := parts[0]

	for _, allowed := range t.opts.AllowedCommands {
		if cmdBase == allowed || strings.HasPrefix(cmd, allowed) {
			return true
		}
	}

	return false
}

// Close releases resources associated with the tool.
func (t *Tool) Close() error {
	return t.runner.Close()
}
