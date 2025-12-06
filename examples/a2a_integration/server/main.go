package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	a2atypes "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2agrpc"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/hupe1980/agentmesh/pkg/a2a"
	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

var (
	grpcPort = flag.Int("grpc-port", 9000, "Port for the gRPC A2A server")
	cardPort = flag.Int("card-port", 9001, "Port for the public AgentCard server")
)

// Simple tool for the agent
type GreetingArgs struct {
	Name string `json:"name"`
}

func main() {
	flag.Parse()

	// Create a simple greeting tool
	greetingTool, err := tool.NewFuncTool(
		"greeting",
		"Generate a personalized greeting",
		func(ctx context.Context, args GreetingArgs) (map[string]any, error) {
			return map[string]any{
				"greeting": fmt.Sprintf("Hello, %s! Welcome to AgentMesh via A2A!", args.Name),
			}, nil
		},
	)
	if err != nil {
		log.Fatalf("Failed to create tool: %v", err)
	}

	// Create an AgentMesh ReAct agent
	compiled, err := agent.NewReAct(
		openai.NewModel(),
		agent.WithTools(greetingTool),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Wrap the agent as an A2A executor
	executor := a2a.NewExecutor(compiled)

	// Create the A2A AgentCard
	agentCard := a2a.CreateAgentCard(
		"AgentMesh ReAct Agent",
		"A ReAct reasoning agent powered by AgentMesh, exposed via the A2A protocol",
		fmt.Sprintf("localhost:%d", *grpcPort),
		[]a2atypes.AgentSkill{
			a2a.CreateAgentSkill(
				"react",
				"ReAct Reasoning",
				"Performs reasoning and action cycles to solve problems using available tools",
				[]string{"reasoning", "problem-solving", "tools"},
				[]string{
					"Generate a greeting for Alice",
					"What tools do you have available?",
				},
			),
		},
	)

	// Start both servers concurrently
	var group errgroup.Group

	// Start gRPC server
	group.Go(func() error {
		return startGRPCServer(*grpcPort, executor)
	})

	// Start AgentCard HTTP server
	group.Go(func() error {
		return servePublicCard(*cardPort, agentCard)
	})

	log.Printf("🚀 AgentMesh A2A Server starting...")
	log.Printf("   gRPC server: localhost:%d", *grpcPort)
	log.Printf("   AgentCard:   http://localhost:%d/.well-known/agent-card", *cardPort)

	if err := group.Wait(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func startGRPCServer(port int, executor *a2a.Executor) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// Create A2A request handler
	requestHandler := a2asrv.NewHandler(executor)

	// Wrap in gRPC transport
	grpcHandler := a2agrpc.NewHandler(requestHandler)

	// Create and start gRPC server
	server := grpc.NewServer()
	grpcHandler.RegisterWith(server)

	log.Printf("Starting gRPC A2A server on port %d", port)
	return server.Serve(listener)
}

func servePublicCard(port int, card *a2atypes.AgentCard) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))

	log.Printf("Starting AgentCard server on port %d", port)
	return http.Serve(listener, mux)
}
