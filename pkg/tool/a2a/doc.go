// Package a2a provides tools for integrating external A2A agents into AgentMesh workflows.
//
// This package enables AgentMesh agents to call external Agent-to-Agent (A2A) protocol
// compliant agents as tools, facilitating multi-agent collaboration across different systems.
//
// # Basic Usage
//
// Create an A2A tool that calls an external agent:
//
//	import "github.com/hupe1980/agentmesh/pkg/tool/a2a"
//
//	// Create tool pointing to an external A2A agent
//	tool, err := a2a.NewTool(
//	    ctx,
//	    "https://external-agent.example.com",
//	    "skill-id",
//	)
//
//	// Use in your agent
//	agent, err := agent.NewReActAgent(
//	    model,
//	    []tool.Tool{tool},
//	)
//
// # Custom Configuration
//
// Configure the A2A client with custom options:
//
//	tool, err := a2a.NewTool(
//	    ctx,
//	    "https://external-agent.example.com",
//	    "skill-id",
//	    a2a.WithName("custom_tool_name"),
//	    a2a.WithDescription("Custom description"),
//	    a2aclient.WithGRPCTransport(grpc.WithTransportCredentials(...)),
//	)
//
// # A2A Protocol
//
// For more information about the A2A protocol, visit https://a2a-protocol.org
//
// # Related Packages
//
// For hosting AgentMesh agents as A2A protocol servers, see:
//   - pkg/a2a: Server infrastructure for exposing AgentMesh agents via A2A protocol
//
// This package (pkg/tool/a2a) focuses on calling external A2A agents as tools,
// while pkg/a2a provides the server-side integration.
package a2a
