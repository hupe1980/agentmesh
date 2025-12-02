package viz

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/internal/safego"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/event"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

// generateRunID creates a unique run identifier using cryptographic randomness.
func generateRunID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

//go:embed ui/static/*
var staticFiles embed.FS

// getStaticFS returns the filesystem for static files
func getStaticFS() (fs.FS, error) {
	return fs.Sub(staticFiles, "ui/static")
}

// Server provides graph visualization and debugging capabilities.
// It serves a web UI for monitoring graph execution, viewing events, and debugging state.
type Server struct {
	config             Config
	registry           *Registry
	eventStore         *EventStore
	wsHub              *WebSocketHub
	httpServer         *http.Server
	testRunner         *TestRunner
	analyticsCollector *AnalyticsCollector

	mu                   sync.RWMutex
	activeRuns           map[string]*RunController
	executionControllers map[string]*ExecutionController
	graphStateTrackers   map[string]*GraphStateTracker
	testSuites           map[string]*TestSuite
	testRuns             map[string]*TestRun
}

// Config configures the visualization server.
type Config struct {
	Addr            string                  // HTTP listen address (default: ":8080")
	EventBufferSize int                     // Maximum events per run (default: 10000)
	Checkpointer    checkpoint.Checkpointer // Checkpoint storage for time-travel debugging
	StaticDir       string                  // Custom UI directory (optional, for development)
}

// NewServer creates a new visualization server with the given configuration.
func NewServer(config Config) (*Server, error) {
	// Apply defaults
	if config.Addr == "" {
		config.Addr = ":8080"
	}
	if config.EventBufferSize == 0 {
		config.EventBufferSize = 10000
	}
	if config.Checkpointer == nil {
		config.Checkpointer = checkpoint.NewInMemoryCheckpointer()
	}

	server := &Server{
		config:               config,
		registry:             NewRegistry(),
		eventStore:           NewEventStore(10000),
		wsHub:                NewWebSocketHub(),
		activeRuns:           make(map[string]*RunController),
		executionControllers: make(map[string]*ExecutionController),
		graphStateTrackers:   make(map[string]*GraphStateTracker),
		testSuites:           make(map[string]*TestSuite),
		testRuns:             make(map[string]*TestRun),
	}

	server.testRunner = NewTestRunner(server)
	server.analyticsCollector = NewAnalyticsCollector(server.eventStore)

	// Setup HTTP routes
	mux := http.NewServeMux()
	server.setupRoutes(mux)
	server.httpServer = &http.Server{
		Addr:              config.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return server, nil
}

// Start starts the HTTP and WebSocket servers.
// It blocks until the context is canceled or an error occurs.
func (s *Server) Start(ctx context.Context) error {
	// Start WebSocket hub
	go s.wsHub.Run()

	// Start HTTP server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		return s.httpServer.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// Stop gracefully stops the server.
func (s *Server) Stop(ctx context.Context) error {
	s.wsHub.Stop()
	return s.httpServer.Shutdown(ctx)
}

// Register adds a runnable to the server.
// For graphs, use: server.Register("my-graph", viz.NewGraphAdapter(graph))
// For agents, use: server.Register("my-agent", viz.NewMessageAdapter(agent))
func (s *Server) Register(id string, runnable Runnable) error {
	return s.registry.Register(id, runnable)
}

// Unregister removes a runnable from the server.
func (s *Server) Unregister(id string) error {
	return s.registry.Unregister(id)
}

// ExecuteGraph executes a registered graph with visualization enabled.
// Returns the run ID for tracking execution progress.
func (s *Server) ExecuteGraph(ctx context.Context, graphID string, input any) (string, error) {
	runnable, err := s.registry.Get(graphID)
	if err != nil {
		return "", err
	}

	// Generate unique run ID
	runID := generateRunID()

	// Initialize run in event store with graph ID
	s.eventStore.InitRun(runID, graphID)

	// Create run controller
	controller := NewRunController(runID, runnable)
	s.mu.Lock()
	s.activeRuns[runID] = controller
	s.mu.Unlock()

	// Create execution controller for debugging
	executionController := NewExecutionController(ctx, runID)
	s.mu.Lock()
	s.executionControllers[runID] = executionController
	s.mu.Unlock()

	// Attach execution controller to context
	ctx = WithExecutionController(ctx, executionController)

	// Attach event handler to context via event bus with execution interceptor
	handler := NewGraphEventHandler(s, runID)
	interceptor := NewExecutionInterceptor(handler)

	// Store state tracker for API access
	s.mu.Lock()
	s.graphStateTrackers[runID] = handler.stateTracker
	s.mu.Unlock()

	// Subscribe interceptor instead of handler for execution control
	eventBus := event.BusFromContext(ctx)
	if eventBus == nil {
		eventBus = event.NewBus()
		ctx = event.WithBus(ctx, eventBus)
	}
	eventBus.Subscribe(interceptor)

	// Broadcast run started event
	s.wsHub.BroadcastMessage(Message{
		Type:  "lifecycle",
		RunID: runID,
		Data: map[string]any{
			"status": "started",
			"graph":  graphID,
		},
	})

	// Execute graph in background
	// IMPORTANT: Use a detached context so execution isn't cancelled when HTTP request completes
	// Copy values from request context (event bus, execution controller) but don't inherit cancellation
	execCtx := context.WithoutCancel(ctx)

	safego.Go(
		func() error {
			defer func() {
				s.mu.Lock()
				delete(s.activeRuns, runID)
				delete(s.executionControllers, runID)
				delete(s.graphStateTrackers, runID)
				s.mu.Unlock()

				// Stop execution controller
				executionController.Stop()
			}()

			// Convert input to map[string]any
			inputMap, ok := input.(map[string]any)
			if !ok {
				inputMap = map[string]any{"content": fmt.Sprintf("%v", input)}
			}

			// Execute with server options (for graph execution tracking, not agent control)
			runOpts := []graph.RunOption{
				graph.WithRunID(runID),
			}

			// Consume outputs - use detached context so execution continues after HTTP response
			for output, err := range runnable.Execute(execCtx, inputMap, runOpts...) {
				if err != nil {
					// Update status in event store
					_ = s.eventStore.UpdateRunStatus(runID, StatusFailed)

					// Get updated run info
					run, _ := s.eventStore.GetRun(runID)

					// Broadcast failure with run_status type (expected by UI)
					s.wsHub.BroadcastMessage(Message{
						Type:  "run_status",
						RunID: runID,
						Data: map[string]any{
							"run": run,
						},
					})

					// Also broadcast as lifecycle event for compatibility
					s.wsHub.BroadcastMessage(Message{
						Type:  "lifecycle",
						RunID: runID,
						Data: map[string]any{
							"status": "failed",
							"error":  err.Error(),
						},
					})
					return err
				}
				_ = output // Output can be processed or logged here
			}

			// Update status in event store
			_ = s.eventStore.UpdateRunStatus(runID, StatusCompleted)

			// Get updated run info
			run, _ := s.eventStore.GetRun(runID)

			// Broadcast completion with run_status type (expected by UI)
			s.wsHub.BroadcastMessage(Message{
				Type:  "run_status",
				RunID: runID,
				Data: map[string]any{
					"run": run,
				},
			})

			// Also broadcast as lifecycle event for compatibility
			s.wsHub.BroadcastMessage(Message{
				Type:  "lifecycle",
				RunID: runID,
				Data: map[string]any{
					"status": "completed",
				},
			})

			return nil
		},
		func(err error) {
			// Handle panics and errors
			_ = s.eventStore.UpdateRunStatus(runID, StatusFailed)
			s.wsHub.BroadcastMessage(Message{
				Type:  "lifecycle",
				RunID: runID,
				Data: map[string]any{
					"status": "failed",
					"error":  err.Error(),
				},
			})
		},
	)

	return runID, nil
}

// GetEvents retrieves events for a specific run.
func (s *Server) GetEvents(runID string, fromStep int64) ([]ExecutionEvent, error) {
	return s.eventStore.GetEvents(runID, fromStep)
}

// CheckpointLoader returns a checkpoint loader for time-travel debugging.
func (s *Server) CheckpointLoader() *CheckpointLoader {
	return NewCheckpointLoader(s.config.Checkpointer, s.eventStore)
}

// StateDiffer returns a state differ for computing checkpoint diffs.
func (s *Server) StateDiffer() *StateDiffer {
	return NewStateDiffer()
}

// setupRoutes configures HTTP endpoints.
func (s *Server) setupRoutes(mux *http.ServeMux) {
	// Static files
	if s.config.StaticDir != "" {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.config.StaticDir))))
	} else {
		staticFS, _ := fs.Sub(staticFiles, "ui/static")
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}

	// API endpoints
	mux.HandleFunc("/api/graphs", s.handleGraphs)
	mux.HandleFunc("/api/graphs/", s.handleGraph)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRun)
	mux.HandleFunc("/api/tests", s.handleTests)
	mux.HandleFunc("/api/tests/", s.handleTest)
	mux.HandleFunc("/api/golden/", func(w http.ResponseWriter, r *http.Request) {
		// Route to either handleGoldenFiles or handleGoldenFile based on path
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/golden/"), "/")
		switch {
		case len(parts) == 1 && parts[0] != "":
			s.handleGoldenFiles(w, r)
		case len(parts) >= 2:
			s.handleGoldenFile(w, r)
		default:
			http.Error(w, "Invalid golden file path", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/api/analytics/summary", s.handleAnalyticsSummary)
	mux.HandleFunc("/api/analytics/predict", s.handleCostPrediction)
	mux.HandleFunc("/api/openapi.yaml", s.handleOpenAPI)

	// WebSocket
	mux.HandleFunc("/ws", s.handleWebSocket)

	// UI root - serve index.html
	staticFS, err := getStaticFS()
	if err == nil {
		if s.config.StaticDir != "" {
			// Use custom directory for development
			mux.Handle("/", http.FileServer(http.Dir(s.config.StaticDir)))
		} else {
			// Use embedded files
			mux.Handle("/", http.FileServer(http.FS(staticFS)))
		}
	}
}
