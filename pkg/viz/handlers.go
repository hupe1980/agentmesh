package viz

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// writeJSON writes JSON response and logs errors
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// handleRunControl handles cancel operation (pause/resume not yet implemented)
func (s *Server) handleRunControl(w http.ResponseWriter, action string, controller *RunController) {
	switch action {
	case "stop", "cancel":
		controller.Cancel()
		writeJSON(w, map[string]any{"status": "canceled"})
	case "pause", "resume":
		// Not yet implemented - would require graph executor support
		http.Error(w, "Pause/resume not yet implemented", http.StatusNotImplemented)
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
	}
}

// handleGraphs handles GET /api/graphs
func (s *Server) handleGraphs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ids := s.registry.List()

	writeJSON(w, map[string]any{
		"graphs": ids,
	})
}

// handleGraph handles /api/graphs/:id
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	// Extract graph ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/graphs/")
	parts := strings.Split(path, "/")
	graphID := parts[0]

	if graphID == "" {
		http.Error(w, "Graph ID required", http.StatusBadRequest)
		return
	}

	// Check for sub-resources
	if len(parts) > 1 { //nolint:nestif // acceptable complexity for HTTP routing
		switch parts[1] {
		case "run":
			if r.Method == http.MethodPost {
				s.handleGraphRun(w, r, graphID)
				return
			}
		case "pause":
			if r.Method == http.MethodPost {
				s.handleGraphPause(w, r, graphID)
				return
			}
		case "resume":
			if r.Method == http.MethodPost {
				s.handleGraphResume(w, r, graphID)
				return
			}
		case "stop":
			if r.Method == http.MethodPost {
				s.handleGraphStop(w, r, graphID)
				return
			}
		}
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetGraph(w, r, graphID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetGraph retrieves graph information
func (s *Server) handleGetGraph(w http.ResponseWriter, r *http.Request, graphID string) {
	// Check for Mermaid diagram request
	path := strings.TrimPrefix(r.URL.Path, "/api/graphs/"+graphID)
	if strings.HasPrefix(path, "/mermaid") {
		s.handleGetGraphMermaid(w, r, graphID)
		return
	}

	// Get runnable
	runnable, err := s.registry.Get(graphID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Return topology and nodes
	writeJSON(w, map[string]any{
		"id":       graphID,
		"nodes":    runnable.GetNodes(),
		"topology": runnable.GetTopology(),
	})
}

// handleGetGraphMermaid generates a Mermaid diagram for a graph
func (s *Server) handleGetGraphMermaid(w http.ResponseWriter, r *http.Request, graphID string) {
	// Get the graph runnable
	runnable, err := s.registry.Get(graphID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Get direction parameter (default: TD = top-down)
	direction := r.URL.Query().Get("direction")
	if direction == "" {
		direction = "TD"
	}

	// Use built-in MermaidFlowchart method from graph.Compiled
	mermaid := runnable.MermaidFlowchart(direction)

	// Note: highlight parameter is not yet supported by built-in method
	// If needed, it can be added by appending style directives
	highlightParam := r.URL.Query().Get("highlight")
	if highlightParam != "" {
		highlightNodes := strings.Split(highlightParam, ",")
		for _, node := range highlightNodes {
			// Sanitize node name for Mermaid
			nodeID := strings.ReplaceAll(node, " ", "_")
			mermaid += fmt.Sprintf("\n    style %s stroke:#FF0000,stroke-width:3px", nodeID)
		}
	}

	writeJSON(w, map[string]string{
		"mermaid": mermaid,
	})
}

// handleRuns handles GET /api/runs
func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runs := s.eventStore.GetRuns()

	writeJSON(w, map[string]any{
		"runs": runs,
	})
}

// handleRun handles /api/runs/:runID
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	// Extract run ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	parts := strings.Split(path, "/")
	runID := parts[0]

	if runID == "" {
		http.Error(w, "Run ID required", http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check for sub-resources
	if len(parts) > 1 {
		s.handleRunSubResource(w, r, runID, parts)
		return
	}

	// Get run metadata
	run, err := s.eventStore.GetRun(runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, run)
}

// handleRunEvents retrieves events for a run with optional filtering
func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	// Check if filter parameters are provided
	query := r.URL.Query()

	// Build filter from query parameters
	filter := EventFilter{}

	// Parse event types
	if typesStr := query.Get("types"); typesStr != "" {
		typesList := strings.Split(typesStr, ",")
		for _, t := range typesList {
			filter.Types = append(filter.Types, EventType(strings.TrimSpace(t)))
		}
	}

	// Parse nodes
	if nodesStr := query.Get("nodes"); nodesStr != "" {
		filter.Nodes = strings.Split(nodesStr, ",")
		for i := range filter.Nodes {
			filter.Nodes[i] = strings.TrimSpace(filter.Nodes[i])
		}
	}

	// Parse search text
	filter.SearchText = query.Get("search")

	// Parse pagination
	if limitStr := query.Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = limit
		}
	}

	if offsetStr := query.Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = offset
		}
	}

	// Use Query if filters are specified, otherwise use GetEvents for backward compatibility
	var events []ExecutionEvent
	var err error

	if len(filter.Types) > 0 || len(filter.Nodes) > 0 || filter.SearchText != "" || filter.Limit > 0 {
		events, err = s.eventStore.Query(runID, filter)
	} else {
		events, err = s.eventStore.GetEvents(runID, 0)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, map[string]any{
		"events": events,
		"total":  len(events),
	})
}

// handleGraphRun starts graph execution
func (s *Server) handleGraphRun(w http.ResponseWriter, r *http.Request, graphID string) {
	// Parse input from request body (if any)
	var input any
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			// If parsing fails, use empty map as input
			input = map[string]any{"input": "start"}
		}
	}

	// If input is still nil or empty, provide default input
	if input == nil {
		input = map[string]any{"input": "start"}
	}

	runID, err := s.ExecuteGraph(r.Context(), graphID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"run_id": runID,
		"status": "started",
	})
}

// handleGraphPause pauses graph execution
func (s *Server) handleGraphPause(w http.ResponseWriter, r *http.Request, _ string) {
	s.handleGraphControlAction(w, r, "pause")
}

// handleGraphResume resumes graph execution
func (s *Server) handleGraphResume(w http.ResponseWriter, r *http.Request, _ string) {
	s.handleGraphControlAction(w, r, "resume")
}

// handleGraphStop stops graph execution
func (s *Server) handleGraphStop(w http.ResponseWriter, r *http.Request, _ string) {
	s.handleGraphControlAction(w, r, "stop")
}

// handleGraphControlAction handles pause/resume/stop operations
func (s *Server) handleGraphControlAction(w http.ResponseWriter, r *http.Request, action string) {
	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		http.Error(w, "run_id required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	controller, exists := s.activeRuns[runID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	s.handleRunControl(w, action, controller)
}

// handleRunCheckpoints retrieves checkpoint timeline for a run
// GET /api/runs/:runID/checkpoints
// Query params: enhanced=true for rich metadata
func (s *Server) handleRunCheckpoints(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if enhanced metadata is requested
	enhanced := r.URL.Query().Get("enhanced") == "true"

	loader := s.CheckpointLoader()

	if enhanced {
		// Load all checkpoints for enhanced metadata
		checkpoints, err := s.config.Checkpointer.List(r.Context(), runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Convert to enhanced metadata
		enhanced := make([]EnhancedCheckpointMetadata, len(checkpoints))
		for i, cp := range checkpoints {
			enhanced[i] = ConvertCheckpointToMetadata(cp, checkpoints)
		}

		writeJSON(w, map[string]any{
			"runID":       runID,
			"checkpoints": enhanced,
			"count":       len(enhanced),
		})
		return
	}

	// Standard timeline
	timeline, err := loader.GetCheckpointTimeline(r.Context(), runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"runID":       runID,
		"checkpoints": timeline,
		"count":       len(timeline),
	})
}

// handleRunCheckpointAt retrieves a specific checkpoint and optionally computes diff
// GET /api/runs/:runID/checkpoint/:superstep
// Query params:
//   - diff=<superstep> to compute diff with another checkpoint
//   - enhanced=true for rich metadata
//   - include_state=false to exclude full state (default: true)
func (s *Server) handleRunCheckpointAt(w http.ResponseWriter, r *http.Request, runID, superstepStr string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse superstep
	superstep, err := strconv.ParseInt(superstepStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid superstep", http.StatusBadRequest)
		return
	}

	// Parse query params
	enhanced := r.URL.Query().Get("enhanced") == "true"
	includeState := r.URL.Query().Get("include_state") != "false"

	loader := s.CheckpointLoader()
	checkpoint, err := loader.LoadCheckpoint(r.Context(), runID, superstep)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Check if diff is requested
	diffParam := r.URL.Query().Get("diff")
	if diffParam != "" {
		s.handleCheckpointDiff(w, r, runID, checkpoint, diffParam, loader, enhanced)
		return
	}

	// Build response
	response := map[string]any{}

	if enhanced {
		// Load all checkpoints for navigation
		allCheckpoints, _ := s.config.Checkpointer.List(r.Context(), runID)
		response["metadata"] = ConvertCheckpointToMetadata(checkpoint, allCheckpoints)
	}

	if includeState {
		response["checkpoint"] = checkpoint

		// Add snapshot
		differ := s.StateDiffer()
		snapshot := differ.ComputeStateSnapshot(checkpoint)
		response["snapshot"] = snapshot
	} else {
		// Return minimal checkpoint without full state
		response["checkpoint"] = map[string]any{
			"run_id":          checkpoint.RunID,
			"superstep":       checkpoint.Superstep,
			"version":         checkpoint.Version,
			"timestamp":       checkpoint.Timestamp,
			"committed":       checkpoint.Committed,
			"completed_nodes": checkpoint.CompletedNodes,
			"paused_nodes":    checkpoint.PausedNodes,
			"pending_writes":  len(checkpoint.PendingWrites),
		}
	}

	writeJSON(w, response)
}

// handleRunExecutionControl handles POST /api/runs/:id/control
func (s *Server) handleRunExecutionControl(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command string `json:"command"`
		Action  string `json:"action"` // Support both command and action
		Target  int64  `json:"target,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Use action if command is empty (for compatibility)
	if req.Command == "" {
		req.Command = req.Action
	}

	// Get execution controller
	s.mu.RLock()
	controller, exists := s.executionControllers[runID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Run not found or not controllable", http.StatusNotFound)
		return
	}

	// Send command
	cmd := ExecutionCommand(req.Command)

	// For jump_to command, set step mode with target
	if cmd == CommandJumpTo {
		controller.SetStepMode(false, req.Target)
		writeJSON(w, map[string]any{
			"status": "accepted",
			"state":  controller.GetState(),
		})
		return
	}

	// For step command, set step mode
	if cmd == CommandStep || cmd == CommandStepNode {
		controller.SetStepMode(true, 0)
		writeJSON(w, map[string]any{
			"status": "accepted",
			"state":  controller.GetState(),
		})
		return
	}

	if err := controller.SendCommand(cmd); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update state based on command
	switch cmd {
	case CommandPause:
		controller.SetState(StatePaused)
	case CommandResume, CommandContinue:
		controller.SetState(StateRunning)
	case CommandStop:
		controller.Stop()
	}

	writeJSON(w, map[string]any{
		"status": "accepted",
		"state":  controller.GetState(),
	})
}

// handleRunBreakpoints handles GET/POST /api/runs/:id/breakpoints
func (s *Server) handleRunBreakpoints(w http.ResponseWriter, r *http.Request, runID string) {
	// Get execution controller
	s.mu.RLock()
	controller, exists := s.executionControllers[runID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Run not found or not controllable", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// List all breakpoints
		breakpoints := controller.GetBreakpoints()
		writeJSON(w, map[string]any{
			"breakpoints": breakpoints,
		})

	case http.MethodPost:
		// Add breakpoint
		var bp Breakpoint
		if err := json.NewDecoder(r.Body).Decode(&bp); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		controller.AddBreakpoint(&bp)
		writeJSON(w, map[string]any{
			"status":     "created",
			"breakpoint": bp,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRunBreakpoint handles PUT/DELETE /api/runs/:id/breakpoints/:breakpoint_id
func (s *Server) handleRunBreakpoint(w http.ResponseWriter, r *http.Request, runID, breakpointID string) {
	// Get execution controller
	s.mu.RLock()
	controller, exists := s.executionControllers[runID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Run not found or not controllable", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodPut:
		// Enable/disable breakpoint
		var req struct {
			Enabled bool `json:"enabled"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := controller.EnableBreakpoint(breakpointID, req.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		writeJSON(w, map[string]any{
			"status": "updated",
		})

	case http.MethodDelete:
		// Remove breakpoint
		if err := controller.RemoveBreakpoint(breakpointID); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		writeJSON(w, map[string]any{
			"status": "deleted",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRunState handles GET/PUT /api/runs/:id/state
func (s *Server) handleRunState(w http.ResponseWriter, r *http.Request, runID string) {
	switch r.Method {
	case http.MethodGet:
		// Try to get the run from event store first
		run, err := s.eventStore.GetRun(runID)
		if err != nil {
			http.Error(w, "Run not found", http.StatusNotFound)
			return
		}

		// Get execution controller if available (for active runs)
		s.mu.RLock()
		controller, hasController := s.executionControllers[runID]
		tracker, hasTracker := s.graphStateTrackers[runID]
		s.mu.RUnlock()

		// Build response with actual state
		response := map[string]any{
			"run_id": runID,
			"status": run.Status,
		}

		if hasController {
			// Active run - get live state from controller
			node, superstep := controller.GetCurrentPosition()
			response["state"] = controller.GetState()
			response["node"] = node
			response["superstep"] = superstep
		}

		if hasTracker {
			// Get graph visualization state
			snapshot := tracker.GetSnapshot()
			response["graph_state"] = snapshot
		}

		// If no live state, try to get from checkpoints
		if !hasController && !hasTracker {
			response["state"] = StatusCompleted
			response["message"] = "Run completed, state available via checkpoints"
		}

		writeJSON(w, response)

	case http.MethodPut:
		// Modify execution state (for time-travel debugging)
		var req struct {
			Node      string `json:"node,omitempty"`
			Superstep int64  `json:"superstep,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// This would require integration with graph executor to actually
		// jump to a different state. For now, just return not implemented.
		http.Error(w, "State modification not yet implemented", http.StatusNotImplemented)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRunGraphSnapshot handles GET /api/runs/:id/graph/snapshot
func (s *Server) handleRunGraphSnapshot(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get graph state tracker
	s.mu.RLock()
	tracker, exists := s.graphStateTrackers[runID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Run not found or no graph state available", http.StatusNotFound)
		return
	}

	// Get snapshot
	snapshot := tracker.GetSnapshot()

	writeJSON(w, snapshot)
}

// handleTests handles GET /api/tests (list suites) and POST /api/tests (create suite)
func (s *Server) handleTests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List all test suites
		s.mu.RLock()
		suites := make([]*TestSuite, 0, len(s.testSuites))
		for _, suite := range s.testSuites {
			suites = append(suites, suite)
		}
		s.mu.RUnlock()

		writeJSON(w, map[string]any{
			"suites": suites,
		})

	case http.MethodPost:
		// Create new test suite
		var suite TestSuite
		if err := json.NewDecoder(r.Body).Decode(&suite); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		s.testSuites[suite.ID] = &suite
		s.mu.Unlock()

		writeJSON(w, suite)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTest handles /api/tests/:id/*
func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	// Extract test suite ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/tests/")
	parts := strings.Split(path, "/")
	suiteID := parts[0]

	if suiteID == "" {
		http.Error(w, "Suite ID required", http.StatusBadRequest)
		return
	}

	// Route to sub-resources
	if len(parts) > 1 {
		switch parts[1] {
		case "run":
			s.handleTestRun(w, r, suiteID)
		case "history":
			s.handleTestHistory(w, r, suiteID)
		case "compare":
			s.handleTestCompare(w, r, suiteID)
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
		return
	}

	// Get/Update/Delete test suite
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		suite, exists := s.testSuites[suiteID]
		s.mu.RUnlock()

		if !exists {
			http.Error(w, "Suite not found", http.StatusNotFound)
			return
		}

		writeJSON(w, suite)

	case http.MethodPut:
		var suite TestSuite
		if err := json.NewDecoder(r.Body).Decode(&suite); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		suite.ID = suiteID
		s.mu.Lock()
		s.testSuites[suiteID] = &suite
		s.mu.Unlock()

		writeJSON(w, suite)

	case http.MethodDelete:
		s.mu.Lock()
		delete(s.testSuites, suiteID)
		s.mu.Unlock()

		writeJSON(w, map[string]any{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTestRun handles POST /api/tests/:id/run
func (s *Server) handleTestRun(w http.ResponseWriter, r *http.Request, suiteID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get test suite
	s.mu.RLock()
	suite, exists := s.testSuites[suiteID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Suite not found", http.StatusNotFound)
		return
	}

	// Parse request
	var req TestRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req.SuiteID = suiteID

	// Run tests
	ctx := r.Context()
	response, err := s.testRunner.RunSuite(ctx, suite, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Store results
	s.mu.Lock()
	for i := range response.Results {
		s.testRuns[response.Results[i].ID] = &response.Results[i]
	}
	s.mu.Unlock()

	writeJSON(w, response)
}

// handleTestHistory handles GET /api/tests/:id/history
func (s *Server) handleTestHistory(w http.ResponseWriter, r *http.Request, suiteID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get test name from query param
	testName := r.URL.Query().Get("test")
	if testName == "" {
		http.Error(w, "Test name required", http.StatusBadRequest)
		return
	}

	// Get history
	history := s.testRunner.GetHistory(suiteID, testName)
	if history == nil {
		http.Error(w, "No history found", http.StatusNotFound)
		return
	}

	writeJSON(w, history)
}

// handleTestCompare handles POST /api/tests/:id/compare
func (s *Server) handleTestCompare(w http.ResponseWriter, r *http.Request, _ string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req TestComparisonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get test runs
	s.mu.RLock()
	baseRun, baseExists := s.testRuns[req.BaseRunID]
	compareRun, compareExists := s.testRuns[req.CompareRunID]
	s.mu.RUnlock()

	if !baseExists || !compareExists {
		http.Error(w, "Test run not found", http.StatusNotFound)
		return
	}

	// Compare runs
	comparison := s.testRunner.CompareRuns(baseRun, compareRun)

	writeJSON(w, comparison)
}

// stringDiff returns elements in a that are not in b
func stringDiff(a, b []string) []string {
	bMap := make(map[string]bool)
	for _, item := range b {
		bMap[item] = true
	}

	diff := []string{}
	for _, item := range a {
		if !bMap[item] {
			diff = append(diff, item)
		}
	}
	return diff
}

// handleGoldenFiles handles GET/POST /api/golden/:suiteID
func (s *Server) handleGoldenFiles(w http.ResponseWriter, r *http.Request) {
	// Extract suite ID from path
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/golden/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Suite ID required", http.StatusBadRequest)
		return
	}
	suiteID := parts[0]

	switch r.Method {
	case http.MethodGet:
		// List golden files for suite
		tests, err := s.testRunner.goldenManager.List(suiteID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list golden files: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"suite_id": suiteID,
			"tests":    tests,
			"count":    len(tests),
		})

	case http.MethodDelete:
		// Delete all golden files for suite (use with caution)
		tests, err := s.testRunner.goldenManager.List(suiteID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list golden files: %v", err), http.StatusInternalServerError)
			return
		}

		deleted := 0
		for _, testName := range tests {
			if err := s.testRunner.goldenManager.Delete(suiteID, testName); err != nil {
				log.Printf("Failed to delete golden file %s/%s: %v", suiteID, testName, err)
			} else {
				deleted++
			}
		}

		writeJSON(w, map[string]any{
			"suite_id": suiteID,
			"deleted":  deleted,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGoldenFile handles GET/PUT/DELETE /api/golden/:suiteID/:testName
func (s *Server) handleGoldenFile(w http.ResponseWriter, r *http.Request) {
	// Extract suite ID and test name from path
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/golden/"), "/")
	if len(parts) < 2 {
		http.Error(w, "Suite ID and test name required", http.StatusBadRequest)
		return
	}
	suiteID := parts[0]
	testName := parts[1]

	switch r.Method {
	case http.MethodGet:
		// Load golden file
		output, err := s.testRunner.goldenManager.Load(suiteID, testName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to load golden file: %v", err), http.StatusNotFound)
			return
		}

		writeJSON(w, map[string]any{
			"suite_id":  suiteID,
			"test_name": testName,
			"output":    output,
			"path":      s.testRunner.goldenManager.GetPath(suiteID, testName),
		})

	case http.MethodPut:
		// Update golden file
		var req struct {
			Output map[string]any `json:"output"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := s.testRunner.goldenManager.Update(suiteID, testName, req.Output); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update golden file: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"suite_id":  suiteID,
			"test_name": testName,
			"status":    "updated",
		})

	case http.MethodDelete:
		// Delete golden file
		if err := s.testRunner.goldenManager.Delete(suiteID, testName); err != nil {
			http.Error(w, fmt.Sprintf("Failed to delete golden file: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"suite_id":  suiteID,
			"test_name": testName,
			"status":    "deleted",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRunAnalytics handles GET /api/runs/:id/analytics
func (s *Server) handleRunAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract run ID from path
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/runs/"), "/")
	if len(parts) < 2 || parts[1] != "analytics" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	runID := parts[0]

	// Try to get cached analytics first
	analytics := s.analyticsCollector.GetRunAnalytics(runID)

	// If not cached, collect it
	if analytics == nil {
		var err error
		analytics, err = s.analyticsCollector.CollectRunAnalytics(runID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to collect analytics: %v", err), http.StatusInternalServerError)
			return
		}
	}

	if analytics == nil {
		http.Error(w, "No analytics available for this run", http.StatusNotFound)
		return
	}

	writeJSON(w, analytics)
}

// handleRunCostBreakdown handles GET /api/runs/:id/costs
func (s *Server) handleRunCostBreakdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract run ID from path
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/runs/"), "/")
	if len(parts) < 2 || parts[1] != "costs" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	runID := parts[0]

	// Ensure analytics exist
	if s.analyticsCollector.GetRunAnalytics(runID) == nil {
		if _, err := s.analyticsCollector.CollectRunAnalytics(runID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to collect analytics: %v", err), http.StatusInternalServerError)
			return
		}
	}

	breakdown := s.analyticsCollector.GetCostBreakdown(runID)
	if breakdown == nil {
		http.Error(w, "No cost data available", http.StatusNotFound)
		return
	}

	writeJSON(w, breakdown)
}

// handleAnalyticsSummary handles GET /api/analytics/summary
func (s *Server) handleAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := AnalyticsQuery{}

	if graphID := r.URL.Query().Get("graph_id"); graphID != "" {
		query.GraphID = graphID
	}

	if status := r.URL.Query().Get("status"); status != "" {
		query.Status = status
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			query.Limit = limit
		}
	}

	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		if startTime, err := time.Parse(time.RFC3339, startStr); err == nil {
			query.StartTime = &startTime
		}
	}

	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endStr); err == nil {
			query.EndTime = &endTime
		}
	}

	summary := s.analyticsCollector.GenerateSummary(query)
	writeJSON(w, summary)
}

// handleCostPrediction handles GET /api/analytics/predict
func (s *Server) handleCostPrediction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	graphID := r.URL.Query().Get("graph_id")

	prediction := s.analyticsCollector.PredictCost(graphID)
	if prediction == nil {
		http.Error(w, "Insufficient data for prediction", http.StatusNotFound)
		return
	}

	writeJSON(w, prediction)
}

// handleOpenAPI serves the OpenAPI specification
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if _, err := w.Write([]byte(OpenAPISpec())); err != nil {
		// Response already started, can't send error
		return
	}
}

// handleRunSubResource routes sub-resource requests to appropriate handlers
func (s *Server) handleRunSubResource(w http.ResponseWriter, r *http.Request, runID string, parts []string) {
	switch parts[1] {
	case "events":
		s.handleRunEvents(w, r, runID)
	case "checkpoints":
		s.handleRunCheckpoints(w, r, runID)
	case "checkpoint":
		if len(parts) > 2 {
			s.handleRunCheckpointAt(w, r, runID, parts[2])
		} else {
			http.Error(w, "Superstep required", http.StatusBadRequest)
		}
	case "control":
		s.handleRunExecutionControl(w, r, runID)
	case "breakpoints":
		if len(parts) > 2 {
			s.handleRunBreakpoint(w, r, runID, parts[2])
		} else {
			s.handleRunBreakpoints(w, r, runID)
		}
	case "state":
		s.handleRunState(w, r, runID)
	case "analytics":
		s.handleRunAnalytics(w, r)
	case "costs":
		s.handleRunCostBreakdown(w, r)
	case "graph":
		if len(parts) > 2 && parts[2] == "snapshot" {
			s.handleRunGraphSnapshot(w, r, runID)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// handleCheckpointDiff handles checkpoint diff requests
func (s *Server) handleCheckpointDiff(w http.ResponseWriter, r *http.Request, runID string, checkpoint *checkpoint.Checkpoint, diffParam string, loader *CheckpointLoader, enhanced bool) {
	diffSuperstep, err := strconv.ParseInt(diffParam, 10, 64)
	if err != nil {
		http.Error(w, "Invalid diff superstep", http.StatusBadRequest)
		return
	}

	otherCheckpoint, err := loader.LoadCheckpoint(r.Context(), runID, diffSuperstep)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load checkpoint at %d: %v", diffSuperstep, err), http.StatusNotFound)
		return
	}

	// Compute diff
	differ := s.StateDiffer()
	diff, err := differ.ComputeDiff(otherCheckpoint, checkpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build diff response
	diffResponse := CheckpointDiffResponse{
		FromSuperstep: otherCheckpoint.Superstep,
		ToSuperstep:   checkpoint.Superstep,
		StateDiffs:    diff.StateDiffs,
		Summary: DiffSummary{
			AddedKeys:     countDiffsByType(diff.StateDiffs, DiffTypeAdded),
			RemovedKeys:   countDiffsByType(diff.StateDiffs, DiffTypeRemoved),
			ModifiedKeys:  countDiffsByType(diff.StateDiffs, DiffTypeModified),
			WritesApplied: diff.WritesApplied,
		},
	}

	// Calculate nodes added/removed
	diffResponse.Summary.NodesAdded = stringDiff(checkpoint.CompletedNodes, otherCheckpoint.CompletedNodes)
	diffResponse.Summary.NodesRemoved = stringDiff(otherCheckpoint.CompletedNodes, checkpoint.CompletedNodes)

	response := map[string]any{
		"from_checkpoint": otherCheckpoint.Superstep,
		"to_checkpoint":   checkpoint.Superstep,
		"diff":            diffResponse,
	}

	if enhanced {
		// Load all checkpoints for navigation
		allCheckpoints, _ := s.config.Checkpointer.List(r.Context(), runID)
		response["metadata"] = ConvertCheckpointToMetadata(checkpoint, allCheckpoints)
	}

	writeJSON(w, response)
}
