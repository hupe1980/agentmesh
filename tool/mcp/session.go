package mcp

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 is fine for this non-cryptographic purpose

	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// headerInjector is an http.RoundTripper that injects custom headers into every request.
// Use this to add authentication or other metadata to outbound MCP HTTP/S requests.
type headerInjector struct {
	Base    http.RoundTripper
	Headers map[string]string
}

// RoundTrip implements the RoundTripper interface.
func (h *headerInjector) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid modifying the original
	newReq := req.Clone(req.Context())
	for k, v := range h.Headers {
		newReq.Header.Set(k, v)
	}

	return h.Base.RoundTrip(newReq)
}

// SessionFactoryOptions controls how a SessionFactory creates an MCP session..
type SessionFactoryOptions struct {
	// MCPClient allows overriding the default MCP client instance.
	MCPClient *mcp.Client
	// Headers are merged into transport-specific headers (for HTTP/S transports).
	Headers map[string]string
}

// HTTPOptions configures HTTP-based transports (Streamable/SSE).
// Provide these via NewStreamableSessionFactory/NewSSESessionFactory option fns.
type HTTPOptions struct {
	// Timeout used by the underlying HTTP client.
	Timeout time.Duration
	// Headers applied to every request from the transport (can be augmented by SessionFactoryOptions.Headers).
	Headers map[string]string
	// BaseTransport allows injecting a custom RoundTripper (e.g., for proxies or custom TLS).
	BaseTransport http.RoundTripper
}

// SessionFactory creates and connects an *mcp.ClientSession* using the
// provided ReadonlyContext and configurable options. Implementations below
// demonstrate stdio (local command), in-memory, and HTTP/S (streamable, SSE)
// transports. Additional transports can be added following this pattern.
//
// The variadic optFns parameter allows callers to customize SessionFactoryOptions
// per session (e.g., to inject authentication headers or override the MCP client).
type SessionFactory func(
	ctx context.Context,
	roCtx core.ReadonlyContext,
	optFns ...func(o *SessionFactoryOptions),
) (*mcp.ClientSession, error)

// NewInMemorySessionFactory returns a SessionFactory that connects using an
// in-memory transport. Useful for tests or in-process MCP servers.
func NewInMemorySessionFactory(transport *mcp.InMemoryTransport) SessionFactory {
	return func(
		ctx context.Context,
		roCtx core.ReadonlyContext,
		optFns ...func(o *SessionFactoryOptions),
	) (*mcp.ClientSession, error) {
		opts := SessionFactoryOptions{
			MCPClient: mcp.NewClient(&mcp.Implementation{Name: "mcp-client", Version: "v1.0.0"}, nil),
		}

		for _, fn := range optFns {
			fn(&opts)
		}

		session, err := opts.MCPClient.Connect(ctx, transport, nil)
		if err != nil {
			return nil, fmt.Errorf("in-memory session init failed: %w", err)
		}

		return session, nil
	}
}

// NewStdioSessionFactory returns a SessionFactory that starts a local MCP
// server via stdio (command + args) and connects to it.
func NewStdioSessionFactory(command string, args ...string) SessionFactory {
	return func(
		ctx context.Context,
		roCtx core.ReadonlyContext,
		optFns ...func(o *SessionFactoryOptions),
	) (*mcp.ClientSession, error) {
		opts := SessionFactoryOptions{
			MCPClient: mcp.NewClient(&mcp.Implementation{Name: "mcp-client", Version: "v1.0.0"}, nil),
		}

		for _, fn := range optFns {
			fn(&opts)
		}

		cmd := exec.CommandContext(ctx, command, args...)

		transport := &mcp.CommandTransport{
			Command: cmd,
		}

		session, err := opts.MCPClient.Connect(ctx, transport, nil)
		if err != nil {
			return nil, fmt.Errorf("stdio session init failed: %w", err)
		}

		return session, nil
	}
}

// buildHTTPClient constructs an http.Client from HTTPOptions and headers.
func buildHTTPClient(httpOpts HTTPOptions, headers map[string]string) *http.Client {
	return &http.Client{
		Timeout:   httpOpts.Timeout,
		Transport: &headerInjector{Base: httpOpts.BaseTransport, Headers: headers},
	}
}

// NewStreamableSessionFactory returns a SessionFactory that connects to an
// MCP server via HTTP using a streamable transport. Custom headers can be
// provided for authentication or other purposes via HTTPOptions and/or per-call
// SessionFactoryOptions in CreateSession.
func NewStreamableSessionFactory(endpoint string, optFns ...func(o *HTTPOptions)) SessionFactory {
	return newHTTPSessionFactory(endpoint,
		func(endpoint string, client *http.Client) mcp.Transport {
			return &mcp.StreamableClientTransport{
				Endpoint:   endpoint,
				HTTPClient: client,
			}
		},
		optFns...,
	)
}

// NewSSESessionFactory returns a SessionFactory that connects to an MCP server
// using Server-Sent Events (SSE). Headers from HTTPOptions and per-call
// SessionFactoryOptions are applied to requests.
func NewSSESessionFactory(endpoint string, optFns ...func(o *HTTPOptions)) SessionFactory {
	return newHTTPSessionFactory(endpoint,
		func(endpoint string, client *http.Client) mcp.Transport {
			return &mcp.SSEClientTransport{
				Endpoint:   endpoint,
				HTTPClient: client,
			}
		},
		optFns...,
	)
}

// newHTTPSessionFactory is a generic factory helper for Streamable or SSE transports
func newHTTPSessionFactory(
	endpoint string,
	transportBuilder func(endpoint string, client *http.Client) mcp.Transport,
	optFns ...func(o *HTTPOptions),
) SessionFactory {
	// Apply HTTP options
	httpOpts := HTTPOptions{
		Timeout:       30 * time.Second,
		BaseTransport: http.DefaultTransport,
	}
	for _, fn := range optFns {
		fn(&httpOpts)
	}

	return func(
		ctx context.Context,
		roCtx core.ReadonlyContext,
		optFns ...func(o *SessionFactoryOptions),
	) (*mcp.ClientSession, error) {
		opts := SessionFactoryOptions{
			MCPClient: mcp.NewClient(&mcp.Implementation{Name: "mcp-client", Version: "v1.0.0"}, nil),
		}
		for _, fn := range optFns {
			fn(&opts)
		}

		headers := mergeHeaders(opts.Headers, httpOpts.Headers)
		client := buildHTTPClient(httpOpts, headers)

		transport := transportBuilder(endpoint, client)

		session, err := opts.MCPClient.Connect(ctx, transport, nil)
		if err != nil {
			return nil, fmt.Errorf("http session init failed: %w", err)
		}

		return session, nil
	}
}

// mergeHeaders merges a base set (optional) with additional headers and
// returns a new map. Callers can pass nil for either argument.
func mergeHeaders(base map[string]string, additional map[string]string) map[string]string {
	if base == nil && additional == nil {
		return nil
	}

	out := make(map[string]string)

	maps.Copy(out, base)
	maps.Copy(out, additional)

	return out
}

// SessionManager manages pooled MCP client sessions keyed by a deterministic
// hash of headers used for transport configuration. Sessions are reused when
// possible and recreated when terminated, similar in spirit to the Python ADK.
//
// Thread-safe. Use Close to gracefully shut down all pooled sessions.
type SessionManager struct {
	factory  SessionFactory
	mu       sync.Mutex
	sessions map[string]*sessionEntry
}

type sessionEntry struct {
	session *mcp.ClientSession
	closed  chan struct{}
}

// NewSessionManager creates a session manager which will use the
// provided factory to create new ClientSessions.
func NewSessionManager(factory SessionFactory) *SessionManager {
	return &SessionManager{
		factory:  factory,
		sessions: make(map[string]*sessionEntry),
	}
}

// CreateSession returns an initialized *mcp.ClientSession*. It reuses a pooled
// session based on a stable key derived from the provided headers. If no
// matching session exists, a new one is created using the configured factory.
// The headers are also forwarded to the factory via SessionFactoryOptions so
// transports can apply them (e.g., for HTTP authentication).
func (m *SessionManager) CreateSession(
	ctx context.Context,
	roCtx core.ReadonlyContext,
	headers map[string]string,
) (*mcp.ClientSession, error) {
	key, err := sessionKeyFromHeaders(headers)
	if err != nil {
		return nil, fmt.Errorf("failed to compute session key: %w", err)
	}

	m.mu.Lock()

	// Check for existing session
	entry, ok := m.sessions[key]
	if ok {
		select {
		case <-entry.closed:
			// closed -> continue to create new
		default:
			sess := entry.session
			m.mu.Unlock()

			return sess, nil
		}
	}

	m.mu.Unlock()

	// create a new session via factory
	sess, err := m.factory(ctx, roCtx, func(o *SessionFactoryOptions) {
		o.Headers = headers
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	entry = &sessionEntry{session: sess, closed: make(chan struct{})}

	// Observe session termination so future calls can recreate as needed.
	go func(e *sessionEntry) {
		// ClientSession.Wait() returns when the session terminates.
		// If your go-sdk version doesn't expose Wait, replace this with a
		// suitable detection mechanism (e.g., polling a closed flag).
		_ = sess.Wait()
		close(e.closed)
	}(entry)

	m.mu.Lock()
	m.sessions[key] = entry
	m.mu.Unlock()

	return sess, nil
}

// sessionKeyFromHeaders returns a deterministic key for a headers map used to
// index pooled sessions. The key is derived by JSON-encoding the map and
// hashing it with MD5 (sufficient for non-cryptographic identity).
func sessionKeyFromHeaders(headers map[string]string) (string, error) {
	if len(headers) == 0 {
		return "session_no_headers", nil
	}

	b, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}

	h := md5.Sum(b) //nolint:gosec // MD5 is fine for this non-cryptographic purpose

	return "session_" + hex.EncodeToString(h[:]), nil
}

// Close closes all pooled sessions and clears the pool.
func (m *SessionManager) Close(ctx context.Context) error {
	m.mu.Lock()
	entries := make([]*sessionEntry, 0, len(m.sessions))
	for _, e := range m.sessions {
		entries = append(entries, e)
	}

	m.sessions = make(map[string]*sessionEntry)

	m.mu.Unlock()

	var aggErr error
	for _, e := range entries {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := e.session.Close()
		select {
		case <-e.closed:
			// closed OK
		case <-cctx.Done():
			if err == nil {
				err = errors.New("timeout waiting for session close")
			}
		}

		cancel()

		if err != nil {
			if aggErr == nil {
				aggErr = err
			} else {
				aggErr = fmt.Errorf("%v; %w", aggErr, err)
			}
		}
	}

	return aggErr
}
