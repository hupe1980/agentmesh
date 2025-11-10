package wasm

import "time"

// SandboxPolicy defines resource access controls for WASM tool execution.
// All fields default to deny-all for security.
type SandboxPolicy struct {
	// Filesystem controls
	AllowFilesystem bool
	AllowedPaths    []string // Whitelist of allowed paths
	ReadOnlyPaths   []string // Paths that can only be read
	MaxFileSize     int64    // Maximum file read/write size

	// Network controls
	AllowNetwork   bool
	AllowedHosts   []string // Whitelist of allowed hosts/IPs
	AllowedPorts   []int    // Whitelist of allowed ports
	MaxRequestSize int64    // Maximum HTTP request/response size

	// Process controls (not applicable to WASM but kept for interface compatibility)
	AllowExec       bool
	AllowedCommands []string

	// Resource limits
	Timeout        time.Duration // Execution timeout
	MaxMemory      int64         // Memory limit in bytes (WASM linear memory)
	MaxMemoryPages uint32        // WASM memory pages (1 page = 64KB)
	MaxStackSize   int64         // Maximum stack depth

	// I/O controls
	AllowStdout   bool  // Allow stdout output
	AllowStderr   bool  // Allow stderr output
	AllowStdin    bool  // Allow stdin input
	MaxOutputSize int64 // Maximum stdout/stderr combined size

	// System capabilities
	AllowRandom      bool // Allow random number generation
	AllowTime        bool // Allow time/date queries
	AllowSleep       bool // Allow sleep/delays
	AllowEnvironment bool // Allow reading environment variables
	AllowLogging     bool // Allow debug logging

	// Determinism controls
	Deterministic  bool       // Force deterministic execution
	FixedTimestamp *time.Time // Fixed time for deterministic runs
	FixedRandSeed  int64      // Fixed random seed

	// WASM-specific controls
	MaxTableSize    uint32 // Limit indirect call table size
	MaxGlobals      uint32 // Limit number of global variables
	MaxFunctions    uint32 // Limit number of functions
	AllowMultiValue bool   // Allow multi-value returns
	AllowBulkMemory bool   // Allow bulk memory operations
	AllowThreads    bool   // Allow threading (if supported)

	// Observability
	LogViolations bool // Log policy violations
	DebugMode     bool // Enable detailed tracing
}

// DefaultSandboxPolicy returns a secure default policy that denies all access.
// This is the recommended starting point - explicitly enable only what you need.
func DefaultSandboxPolicy() *SandboxPolicy {
	return &SandboxPolicy{
		// Deny all external access
		AllowFilesystem: false,
		AllowNetwork:    false,
		AllowExec:       false,

		// Deny all I/O
		AllowStdout: false,
		AllowStderr: false,
		AllowStdin:  false,

		// Deny system capabilities
		AllowRandom:      false,
		AllowTime:        false,
		AllowSleep:       false,
		AllowEnvironment: false,
		AllowLogging:     false,

		// Conservative resource limits
		Timeout:        5 * time.Second,
		MaxMemory:      100 * 1024 * 1024, // 100MB
		MaxMemoryPages: 1600,              // 100MB / 64KB
		MaxStackSize:   512 * 1024,        // 512KB
		MaxOutputSize:  1 * 1024 * 1024,   // 1MB

		// WASM limits
		MaxTableSize:    1000,
		MaxGlobals:      100,
		MaxFunctions:    10000,
		AllowMultiValue: true,
		AllowBulkMemory: false,
		AllowThreads:    false,

		// Observability
		LogViolations: true,
		DebugMode:     false,
	}
}

// ComputeOnlyPolicy returns a policy for pure computation (math, crypto, parsing).
// Allows computation with memory and random access but no I/O.
func ComputeOnlyPolicy() *SandboxPolicy {
	policy := DefaultSandboxPolicy()
	policy.Timeout = 10 * time.Second
	policy.MaxMemory = 200 * 1024 * 1024 // 200MB for data processing
	policy.MaxMemoryPages = 3200         // 200MB / 64KB
	policy.AllowRandom = true            // Allow crypto-grade randomness
	return policy
}

// NetworkOnlyPolicy returns a policy for API clients (no filesystem).
// Allows network access to specified hosts only.
func NetworkOnlyPolicy(allowedHosts ...string) *SandboxPolicy {
	policy := DefaultSandboxPolicy()
	policy.AllowNetwork = true
	policy.AllowedHosts = allowedHosts
	policy.AllowStdout = true                // Allow logging HTTP requests
	policy.Timeout = 30 * time.Second        // Longer timeout for network I/O
	policy.MaxRequestSize = 10 * 1024 * 1024 // 10MB request/response
	policy.MaxMemory = 150 * 1024 * 1024     // 150MB
	policy.MaxOutputSize = 5 * 1024 * 1024   // 5MB logs
	return policy
}

// FileProcessingPolicy returns a policy for file manipulation tools.
// Allows filesystem access to specified paths only.
func FileProcessingPolicy(allowedPaths []string, readOnly bool) *SandboxPolicy {
	policy := DefaultSandboxPolicy()
	policy.AllowFilesystem = true
	policy.AllowedPaths = allowedPaths
	if readOnly {
		policy.ReadOnlyPaths = allowedPaths
	}
	policy.AllowStdout = true
	policy.MaxFileSize = 100 * 1024 * 1024 // 100MB max file size
	policy.MaxMemory = 500 * 1024 * 1024   // 500MB for file buffering
	policy.MaxMemoryPages = 8000           // 500MB / 64KB
	policy.Timeout = 60 * time.Second      // 1 minute for file processing
	return policy
}

// DeterministicPolicy returns a policy for reproducible execution.
// Forces deterministic behavior for testing and verification.
func DeterministicPolicy() *SandboxPolicy {
	policy := DefaultSandboxPolicy()
	policy.Deterministic = true
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	policy.FixedTimestamp = &fixedTime
	policy.FixedRandSeed = 42
	policy.AllowRandom = true // But with fixed seed
	policy.AllowTime = true   // But with fixed timestamp
	return policy
}

// PermissiveSandboxPolicy returns a policy with minimal restrictions.
// WARNING: Only use for trusted internal tools.
func PermissiveSandboxPolicy() *SandboxPolicy {
	return &SandboxPolicy{
		AllowFilesystem:  true,
		AllowNetwork:     true,
		AllowExec:        true,
		AllowStdout:      true,
		AllowStderr:      true,
		AllowRandom:      true,
		AllowTime:        true,
		AllowSleep:       true,
		AllowEnvironment: true,
		AllowLogging:     true,
		Timeout:          30 * time.Second,
		MaxMemory:        1024 * 1024 * 1024, // 1GB
		MaxMemoryPages:   16384,              // 1GB / 64KB
		MaxOutputSize:    10 * 1024 * 1024,   // 10MB
		MaxFileSize:      1024 * 1024 * 1024, // 1GB
		MaxRequestSize:   100 * 1024 * 1024,  // 100MB
		MaxTableSize:     10000,
		MaxGlobals:       1000,
		MaxFunctions:     100000,
		AllowMultiValue:  true,
		AllowBulkMemory:  true,
		AllowThreads:     false, // Threading rarely needed
		LogViolations:    true,
		DebugMode:        false,
	}
}

// Clone creates a deep copy of the SandboxPolicy.
func (p *SandboxPolicy) Clone() *SandboxPolicy {
	clone := *p

	// Deep copy slices
	if p.AllowedPaths != nil {
		clone.AllowedPaths = make([]string, len(p.AllowedPaths))
		copy(clone.AllowedPaths, p.AllowedPaths)
	}
	if p.ReadOnlyPaths != nil {
		clone.ReadOnlyPaths = make([]string, len(p.ReadOnlyPaths))
		copy(clone.ReadOnlyPaths, p.ReadOnlyPaths)
	}
	if p.AllowedHosts != nil {
		clone.AllowedHosts = make([]string, len(p.AllowedHosts))
		copy(clone.AllowedHosts, p.AllowedHosts)
	}
	if p.AllowedPorts != nil {
		clone.AllowedPorts = make([]int, len(p.AllowedPorts))
		copy(clone.AllowedPorts, p.AllowedPorts)
	}
	if p.AllowedCommands != nil {
		clone.AllowedCommands = make([]string, len(p.AllowedCommands))
		copy(clone.AllowedCommands, p.AllowedCommands)
	}

	// Deep copy pointer
	if p.FixedTimestamp != nil {
		t := *p.FixedTimestamp
		clone.FixedTimestamp = &t
	}

	return &clone
}
