// Package docker provides Docker-based tool sandboxing for secure execution
// of containerized commands with resource limits and network isolation.
//
// Docker tools run inside isolated containers, providing access to any
// CLI tool available as a Docker image (nmap, ffmpeg, imagemagick, etc.)
// while maintaining strict security boundaries through Linux namespaces,
// resource limits, and network policies.
//
// # Security Model
//
// Docker tools provide container-level security guarantees:
//   - Network isolation: Default "none" mode blocks all network access
//   - Resource limits: Memory and CPU usage strictly enforced
//   - Filesystem isolation: No host access unless explicitly mounted
//   - Command allowlisting: Optionally restrict which commands can run
//
// # Example Usage
//
//	// Create a network scanning tool using nmap
//	nmapTool, err := docker.NewTool("nmap_scan", "instrumentisto/nmap:latest",
//	    docker.WithDescription("Scan network hosts and ports using nmap"),
//	    docker.WithTimeout(5*time.Minute),
//	    docker.WithNetworkMode("host"),
//	    docker.WithPullImage(true),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer nmapTool.Close()
//
//	// Execute the tool
//	result, err := nmapTool.Call(ctx, `{"command": "-sV -p 80,443 example.com"}`)
//
// # Network Modes
//
// The package supports Docker network modes:
//   - "none": Complete network isolation (default, most secure)
//   - "bridge": Container can access external networks via Docker bridge
//   - "host": Container shares host network stack (least secure, use with caution)
//
// # Resource Limits
//
// Default resource limits applied to all containers:
//   - Memory: 256MB (configurable via WithMemoryLimit)
//   - CPU: 0.5 CPU (configurable via WithCPUQuota)
//   - Timeout: 30 seconds (configurable via WithTimeout)
//
// # Comparison with WASM Tools
//
// Use Docker tools when you need:
//   - Existing CLI tools (nmap, ffmpeg, curl, etc.)
//   - Complex dependencies that would be hard to compile to WASM
//   - Network access (HTTP clients, scanners)
//   - Longer-running tasks where ~100-500ms startup is acceptable
//
// Use WASM tools when you need:
//   - Sub-millisecond latency
//   - Maximum memory isolation
//   - Simple, self-contained logic
//   - No external dependencies
package docker
