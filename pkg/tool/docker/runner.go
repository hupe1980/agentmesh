package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

// Runner provides a reusable interface for running containerized tools.
// It wraps the Docker client and handles container lifecycle management.
type Runner struct {
	client *client.Client
}

// NewRunner creates a new Docker runner instance.
// It connects to the Docker daemon using environment variables (DOCKER_HOST, etc.)
// and automatically negotiates the API version.
func NewRunner() (*Runner, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker: failed to create client: %w", err)
	}

	return &Runner{client: cli}, nil
}

// NewRunnerWithClient creates a new Docker runner with a pre-configured client.
// Use this for testing or when you need custom client configuration.
func NewRunnerWithClient(cli *client.Client) *Runner {
	return &Runner{client: cli}
}

// Config holds the configuration for running a Docker container.
type Config struct {
	// Image is the Docker image to run (e.g., "instrumentisto/nmap:latest")
	Image string

	// Command is the command and arguments to pass to the container
	Command []string

	// Env is a list of environment variables in the form "KEY=value"
	Env []string

	// NetworkMode specifies the network mode ("host", "bridge", "none")
	// Default is "none" for maximum isolation.
	NetworkMode string

	// Timeout for the container execution. Zero means no timeout.
	Timeout time.Duration

	// AutoRemove determines if the container should be removed after execution.
	AutoRemove bool

	// Privileged runs the container in privileged mode.
	// WARNING: Use with extreme caution - bypasses most security features.
	Privileged bool

	// Mounts for volume bindings (optional)
	Mounts []mount.Mount

	// WorkingDir sets the working directory inside the container
	WorkingDir string

	// User sets the user inside the container (e.g., "1000:1000")
	User string

	// PullImage determines if the image should be pulled before running
	PullImage bool

	// MemoryLimit is the memory limit in bytes (0 = unlimited)
	MemoryLimit int64

	// CPUQuota is the CPU quota (100000 = 1 CPU)
	CPUQuota int64
}

// Result contains the output and metadata from a container run.
type Result struct {
	// Stdout contains the standard output from the container
	Stdout []byte

	// Stderr contains the standard error from the container
	Stderr []byte

	// ExitCode is the container's exit code
	ExitCode int

	// Duration is how long the container ran
	Duration time.Duration
}

// Run executes a Docker container with the given configuration and returns the result.
// The container is started, waited for, and its output is captured.
// If AutoRemove is false, the container is forcefully removed after execution.
func (r *Runner) Run(ctx context.Context, config Config) (*Result, error) {
	start := time.Now()

	// Apply timeout if specified
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	// Set defaults
	if config.NetworkMode == "" {
		config.NetworkMode = "none" // Default to isolated networking
	}

	// Pull image if requested
	if config.PullImage {
		if err := r.pullImage(ctx, config.Image); err != nil {
			return nil, fmt.Errorf("docker: failed to pull image: %w", err)
		}
	}

	// Create container
	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(config.NetworkMode),
		AutoRemove:  config.AutoRemove,
		Privileged:  config.Privileged,
		Mounts:      config.Mounts,
		Resources: container.Resources{
			Memory:   config.MemoryLimit,
			CPUQuota: config.CPUQuota,
		},
	}

	resp, err := r.client.ContainerCreate(ctx,
		&container.Config{
			Image:      config.Image,
			Cmd:        config.Command,
			Env:        config.Env,
			WorkingDir: config.WorkingDir,
			User:       config.User,
			Tty:        false,
		},
		hostConfig,
		nil, nil, "",
	)
	if err != nil {
		return nil, fmt.Errorf("docker: failed to create container: %w", err)
	}

	// Ensure cleanup if AutoRemove is false
	if !config.AutoRemove {
		defer func() {
			_ = r.client.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{
				Force: true,
			})
		}()
	}

	// Start container
	if err := r.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("docker: failed to start container: %w", err)
	}

	// Wait for completion
	statusCh, errCh := r.client.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)

	var exitCode int64

	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("docker: container wait error: %w", err)
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	}

	// Get logs
	logs, err := r.client.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("docker: failed to get container logs: %w", err)
	}
	defer func() { _ = logs.Close() }()

	// Docker multiplexes stdout/stderr in the logs stream
	stdout, stderr, err := r.demuxLogs(logs)
	if err != nil {
		return nil, fmt.Errorf("docker: failed to read logs: %w", err)
	}

	return &Result{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: int(exitCode),
		Duration: time.Since(start),
	}, nil
}

// pullImage pulls a Docker image.
func (r *Runner) pullImage(ctx context.Context, imageName string) error {
	reader, err := r.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	// Consume the pull output (required to complete the pull)
	_, err = io.Copy(io.Discard, reader)

	return err
}

// demuxLogs separates stdout and stderr from Docker's multiplexed log stream.
// Docker uses an 8-byte header: [STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4]
func (r *Runner) demuxLogs(logs io.Reader) (stdout, stderr []byte, err error) {
	var stdoutBuf, stderrBuf []byte
	header := make([]byte, 8)

	for {
		_, err := io.ReadFull(logs, header)
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, nil, err
		}

		streamType := header[0]
		size := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])

		data := make([]byte, size)
		if _, err := io.ReadFull(logs, data); err != nil {
			return nil, nil, err
		}

		switch streamType {
		case 1: // stdout
			stdoutBuf = append(stdoutBuf, data...)
		case 2: // stderr
			stderrBuf = append(stderrBuf, data...)
		}
	}

	return stdoutBuf, stderrBuf, nil
}

// ImageExists checks if a Docker image exists locally.
func (r *Runner) ImageExists(ctx context.Context, imageName string) (bool, error) {
	_, err := r.client.ImageInspect(ctx, imageName)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// Close closes the Docker client connection.
func (r *Runner) Close() error {
	if r.client != nil {
		return r.client.Close()
	}

	return nil
}
