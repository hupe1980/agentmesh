// Package mcp provides integration with the Model Context Protocol (MCP).
//
// MCP is a standard protocol for connecting AI models to external tools and data sources.
// This package allows AgentMesh agents to dynamically discover and use tools from MCP servers.
//
// # Dependencies
//
// The MCP package requires the MCP SDK:
//
//	go get github.com/modelcontextprotocol/go-sdk/mcp
//
// # Usage with ReActAgent
//
// Create a toolset that connects to an MCP server:
//
//	import (
//	    "github.com/hupe1980/agentmesh/pkg/agent"
//	    "github.com/hupe1980/agentmesh/pkg/tool/mcp"
//	)
//
//	// Connect to an MCP server via stdio
//	factory := mcp.NewStdioSessionFactory("mcp-server", []string{"--config", "config.json"})
//	toolset := mcp.NewToolset(factory)
//
//	// Use the toolset in a ReAct agent
//	agent, err := agent.NewReActAgent(model,
//	    agent.WithToolset(toolset),
//	    agent.WithMaxIterations(10))
//
// # Transport Options
//
// The package supports multiple MCP transports:
//
//   - Stdio: Connect to a local MCP server process
//   - HTTP Streamable: Connect via HTTP streaming
//   - SSE: Connect via Server-Sent Events
//   - InMemory: For testing or in-process MCP servers
//
// Example with HTTP transport:
//
//	factory := mcp.NewStreamableSessionFactory("https://mcp.example.com/tools")
//	toolset := mcp.NewToolset(factory)
//
// # Session Management
//
// The package automatically manages MCP client sessions, reusing connections
// when possible and recreating them when terminated. Sessions are pooled based
// on their configuration (headers, endpoints, etc.).
package mcp
