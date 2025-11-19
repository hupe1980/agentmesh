package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// RuntimeConfig is ephemeral configuration that shouldn't be checkpointed.
type RuntimeConfig struct {
	APIKey     string
	Timeout    time.Duration
	MaxRetries int
	Debug      bool
}

// SessionInfo tracks user session state (ephemeral, not persisted).
type SessionInfo struct {
	UserID       string
	SessionToken string
	LoginTime    time.Time
}

// MetricsCollector accumulates runtime metrics (not checkpointed).
type MetricsCollector struct {
	NodeExecutions map[string]int
	TotalLatency   time.Duration
}

func (m *MetricsCollector) RecordExecution(nodeName string, latency time.Duration) {
	if m.NodeExecutions == nil {
		m.NodeExecutions = make(map[string]int)
	}
	m.NodeExecutions[nodeName]++
	m.TotalLatency += latency
}

// State keys for persistent data (checkpointed)
var (
	CounterKey  = state.NewKey("counter", 0)
	LastNodeKey = state.NewKey("last_node", "")
	HistoryKey  = state.NewListKey[string]("history", 100)
)

// ConfigurableNode uses managed values for runtime configuration.
type ConfigurableNode struct {
	name    string
	manager *state.Manager
}

func (n *ConfigurableNode) Name() string {
	return n.name
}

func (n *ConfigurableNode) Compute(ctx context.Context, view *state.ReadView) (state.Updates, error) {
	start := time.Now()

	// Access persistent state (checkpointed)
	counter := state.GetFromView(view, CounterKey)
	lastNode := state.GetFromView(view, LastNodeKey)

	// Access managed values (ephemeral, NOT checkpointed)
	config, err := state.GetManagedValue[*RuntimeConfig](n.manager, ctx, "runtime_config")
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	session, err := state.GetManagedValue[*SessionInfo](n.manager, ctx, "session")
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	metrics, err := state.GetManagedValue[*MetricsCollector](n.manager, ctx, "metrics")
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	// Use configuration
	fmt.Printf("\n[%s] Node Configuration:\n", n.name)
	fmt.Printf("  - API Key: %s (masked)\n", maskAPIKey(config.APIKey))
	fmt.Printf("  - Timeout: %v\n", config.Timeout)
	fmt.Printf("  - Max Retries: %d\n", config.MaxRetries)
	fmt.Printf("  - Debug Mode: %v\n", config.Debug)

	// Use session info
	fmt.Printf("\n[%s] Session Info:\n", n.name)
	fmt.Printf("  - User: %s\n", session.UserID)
	fmt.Printf("  - Token: %s (masked)\n", maskAPIKey(session.SessionToken))
	fmt.Printf("  - Session Duration: %v\n", time.Since(session.LoginTime))

	// Use persistent state
	fmt.Printf("\n[%s] Persistent State:\n", n.name)
	fmt.Printf("  - Counter: %d\n", counter)
	fmt.Printf("  - Last Node: %s\n", lastNode)

	// Simulate work with timeout
	workDone := make(chan bool)
	go func() {
		time.Sleep(50 * time.Millisecond)
		workDone <- true
	}()

	select {
	case <-workDone:
		fmt.Printf("\n[%s] Work completed within timeout\n", n.name)
	case <-time.After(config.Timeout):
		return nil, fmt.Errorf("timeout exceeded")
	}

	// Record metrics (updates managed value in-place)
	latency := time.Since(start)
	metrics.RecordExecution(n.name, latency)

	// Return persistent state updates (these WILL be checkpointed)
	return state.Updates{
		CounterKey.Name():  counter + 1,
		LastNodeKey.Name(): n.name,
		HistoryKey.Name():  []string{fmt.Sprintf("%s executed by user %s", n.name, session.UserID)},
	}, nil
}

func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// MetricsNode reads and displays accumulated metrics.
type MetricsNode struct {
	manager *state.Manager
}

func (n *MetricsNode) Name() string {
	return "metrics_reporter"
}

func (n *MetricsNode) Compute(ctx context.Context, view *state.ReadView) (state.Updates, error) {
	// Read metrics (managed value)
	metrics, err := state.GetManagedValue[*MetricsCollector](n.manager, ctx, "metrics")
	if err != nil {
		return nil, err
	}

	fmt.Printf("\n=== Runtime Metrics ===\n")
	fmt.Printf("Total Executions: %d\n", len(metrics.NodeExecutions))
	fmt.Printf("Total Latency: %v\n", metrics.TotalLatency)
	fmt.Printf("\nPer-Node Executions:\n")
	for node, count := range metrics.NodeExecutions {
		fmt.Printf("  - %s: %d times\n", node, count)
	}
	fmt.Printf("======================\n\n")

	return state.Updates{
		HistoryKey.Name(): []string{"Metrics reported"},
	}, nil
}

func main() {
	ctx := context.Background()

	// Create state manager
	mgr := state.NewManager()

	// Register persistent state keys (these ARE checkpointed)
	if err := state.RegisterKey(mgr, CounterKey); err != nil {
		log.Fatal(err)
	}
	if err := state.RegisterKey(mgr, LastNodeKey); err != nil {
		log.Fatal(err)
	}
	if err := state.RegisterListKey(mgr, HistoryKey); err != nil {
		log.Fatal(err)
	}

	// Register managed values (ephemeral, NOT checkpointed)

	// 1. Runtime configuration
	configMV := state.NewManagedValueWithDefault("runtime_config", &RuntimeConfig{
		APIKey:     "sk_live_1234567890abcdef",
		Timeout:    200 * time.Millisecond,
		MaxRetries: 3,
		Debug:      true,
	})
	if err := state.RegisterManagedValue(mgr, configMV); err != nil {
		log.Fatal(err)
	}

	// 2. Session state
	sessionMV := state.NewManagedValueWithDefault("session", &SessionInfo{
		UserID:       "user@example.com",
		SessionToken: "tok_session_abcd1234",
		LoginTime:    time.Now(),
	})
	if err := state.RegisterManagedValue(mgr, sessionMV); err != nil {
		log.Fatal(err)
	}

	// 3. Metrics collector
	metricsMV := state.NewManagedValueWithDefault("metrics", &MetricsCollector{
		NodeExecutions: make(map[string]int),
	})
	if err := state.RegisterManagedValue(mgr, metricsMV); err != nil {
		log.Fatal(err)
	}

	// 4. Computed managed value (always fresh)
	currentTimeMV := state.NewComputedManagedValue("current_time", func(ctx context.Context) (string, error) {
		return time.Now().Format(time.RFC3339), nil
	})
	if err := state.RegisterManagedValue(mgr, currentTimeMV); err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ Registered managed values:")
	for _, name := range mgr.GetManagedValueNames() {
		fmt.Printf("  - %s\n", name)
	}

	// Build graph with nodes that use managed values
	gph, err := graph.NewGraph(mgr)
	if err != nil {
		log.Fatal(err)
	}

	// Add nodes using graph.NewBaseNode
	node1 := &ConfigurableNode{name: "processor_1", manager: mgr}
	node2 := &ConfigurableNode{name: "processor_2", manager: mgr}
	metricsNode := &MetricsNode{manager: mgr}

	if err := gph.AddNode(graph.NewBaseNode(node1.Name(), node1.Compute)); err != nil {
		log.Fatal(err)
	}
	if err := gph.AddNode(graph.NewBaseNode(node2.Name(), node2.Compute)); err != nil {
		log.Fatal(err)
	}
	if err := gph.AddNode(graph.NewBaseNode(metricsNode.Name(), metricsNode.Compute)); err != nil {
		log.Fatal(err)
	}

	// Connect nodes
	gph.AddEdge(graph.StartNode, node1.Name())
	gph.AddEdge(node1.Name(), node2.Name())
	gph.AddEdge(node2.Name(), metricsNode.Name())
	gph.AddEdge(metricsNode.Name(), graph.EndNode)

	// Compile graph
	compiled, err := graph.Compile(gph, graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	// Initialize state
	initialState := state.Updates{
		CounterKey.Name():  0,
		LastNodeKey.Name(): "start",
	}
	if err := mgr.ApplyUpdates(ctx, initialState); err != nil {
		log.Fatal(err)
	}

	// Execute graph
	fmt.Println("\n=== Starting Graph Execution ===\n")

	for result := range compiled.Run(ctx, nil) {
		if result != nil {
			log.Fatal(result)
		}
	}

	// Show final persistent state (this IS checkpointed)
	fmt.Println("\n=== Final Persistent State (Checkpointed) ===")
	finalView, err := mgr.CreateReadView(ctx)
	if err != nil {
		log.Fatal(err)
	}
	counter := state.GetFromView(finalView, CounterKey)
	lastNode := state.GetFromView(finalView, LastNodeKey)
	fmt.Printf("Counter: %v\n", counter)
	fmt.Printf("Last Node: %v\n", lastNode)

	// Show final managed values (NOT checkpointed)
	fmt.Println("\n=== Final Managed Values (Ephemeral, NOT Checkpointed) ===")

	config, _ := state.GetManagedValue[*RuntimeConfig](mgr, ctx, "runtime_config")
	fmt.Printf("Runtime Config: APIKey=%s, Timeout=%v, MaxRetries=%d\n",
		maskAPIKey(config.APIKey), config.Timeout, config.MaxRetries)

	session, _ := state.GetManagedValue[*SessionInfo](mgr, ctx, "session")
	fmt.Printf("Session: User=%s, Duration=%v\n",
		session.UserID, time.Since(session.LoginTime))

	currentTime, _ := state.GetManagedValue[string](mgr, ctx, "current_time")
	fmt.Printf("Current Time (computed): %s\n", currentTime)

	// Demonstrate runtime configuration update (not from nodes)
	fmt.Println("\n=== Updating Runtime Configuration ===")
	newConfig := &RuntimeConfig{
		APIKey:     "sk_live_newkey_xyz789",
		Timeout:    500 * time.Millisecond,
		MaxRetries: 5,
		Debug:      false,
	}
	if err := state.SetManagedValue(mgr, ctx, "runtime_config", newConfig); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Configuration updated (affects next execution)")

	// Key Insight: Checkpointing demonstration
	fmt.Println("\n=== Checkpoint Behavior ===")
	fmt.Println("Persistent State (Counter, History):")
	fmt.Println("  ✓ INCLUDED in checkpoints")
	fmt.Println("  ✓ Survives process restart")
	fmt.Println("  ✓ Used for time travel")
	fmt.Println("\nManaged Values (Config, Session, Metrics):")
	fmt.Println("  ✗ NOT included in checkpoints")
	fmt.Println("  ✗ Lost on process restart")
	fmt.Println("  ✓ Reinitialized at runtime")
	fmt.Println("  ✓ Perfect for ephemeral state")
}
