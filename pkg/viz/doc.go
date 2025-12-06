// Package viz provides real-time visualization and debugging for AgentMesh graphs and agents.
//
// The viz package enables developers to monitor, debug, and understand graph execution through:
//   - Real-time event streaming via WebSocket
//   - Interactive Mermaid graph visualization
//   - Complete execution timeline with filtering
//   - Checkpoint-based time-travel debugging
//   - State diffing between execution points
//
// # Architecture
//
// The package provides clean interfaces for integrating graphs and agents:
//   - viz.Runnable: Main interface for executable components
//   - viz.GraphAdapter: Type-safe wrapper for graph.Compiled instances
//   - viz.MessageAdapter: Wrapper for message-based agents (ReActAgent, etc.)
//   - viz.Server: HTTP/WebSocket server with embedded UI
//   - viz.Registry: Thread-safe storage for registered runnables
//
// # Quick Start
//
//	// Create server
//	server, _ := viz.NewServer(viz.Config{
//	    Addr:            ":8080",
//	    EventBufferSize: 10000,
//	    Checkpointer:    checkpoint.NewInMemoryCheckpointer(),
//	})
//
//	// Register an agent
//	agent, _ := agent.NewReAct(model)
//	server.Register("my-agent", viz.NewMessageAdapter(agent))
//
//	// Start server
//	server.Start(context.Background())
//
// Visit http://localhost:8080 to access the visualization UI.
//
// # Event Capture
//
// Events are automatically captured when executing graphs through the server.
// The server attaches event handlers to the context, enabling:
//   - Real-time visualization updates
//   - Checkpoint creation for time-travel
//   - Complete execution history
//
// # Agent vs Graph Execution
//
// The package correctly handles the distinction between graphs and agents:
//   - GraphAdapter passes execution options to compiled graphs
//   - MessageAdapter does NOT pass server options to agents
//   - Agents are self-contained and manage their own execution
//   - Context propagation enables event capture for both
//
// See examples/viz_server for a complete working example.
package viz
