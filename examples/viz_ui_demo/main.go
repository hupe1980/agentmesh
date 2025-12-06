package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/hupe1980/agentmesh/pkg/viz"
)

// Example demonstrating the new Viz UI with real-time features

// weatherArgs defines the weather tool parameters
type weatherArgs struct {
	Location string `json:"location" jsonschema:"description=The city or location"`
}

// mockWeatherLookup simulates weather API
func mockWeatherLookup(_ context.Context, args weatherArgs) (map[string]any, error) {
	time.Sleep(time.Duration(500+len(args.Location)*10) * time.Millisecond)
	return map[string]any{
		"location":      args.Location,
		"conditions":    "Sunny",
		"temperature_c": 22.0 + float64(len(args.Location)%10),
		"humidity":      65,
	}, nil
}

// timeArgs defines the time tool parameters
type timeArgs struct {
	Timezone string `json:"timezone" jsonschema:"description=Timezone (e.g., UTC, EST)"`
}

// mockGetTime simulates time API
func mockGetTime(_ context.Context, args timeArgs) (map[string]any, error) {
	time.Sleep(300 * time.Millisecond)
	return map[string]any{
		"timezone": args.Timezone,
		"time":     time.Now().Format(time.RFC3339),
		"unix":     time.Now().Unix(),
	}, nil
}

func main() {
	// Get API key
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable required")
	}

	// Create viz server
	server, err := viz.NewServer(viz.Config{
		Addr:            ":8080",
		EventBufferSize: 10000,
		Checkpointer:    checkpoint.NewInMemoryCheckpointer(),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create tools
	weatherTool, _ := tool.NewFuncTool(
		"get_weather",
		"Get current weather for a city",
		mockWeatherLookup,
	)

	timeTool, _ := tool.NewFuncTool(
		"get_time",
		"Get current time in a timezone",
		mockGetTime,
	)

	// Create ReAct agent with tools
	reactAgent, err := agent.NewReAct(
		openai.NewModel(),
		agent.WithTools(weatherTool, timeTool),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Register agent
	if err := server.Register("demo-agent", viz.NewMessageAdapter(reactAgent)); err != nil {
		log.Fatal(err)
	}

	fmt.Println("🚀 AgentMesh Visualization UI Demo")
	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Println("")
	fmt.Println("✨ New UI Features:")
	fmt.Println("  📡 Real-time event streaming with WebSocket")
	fmt.Println("  📊 Live analytics and cost tracking")
	fmt.Println("  🎯 Run selection and filtering")
	fmt.Println("  💾 State inspection and checkpoints")
	fmt.Println("  🧪 Test management interface")
	fmt.Println("  📈 Performance metrics and bottlenecks")
	fmt.Println("")
	fmt.Println("🌐 Open your browser:")
	fmt.Println("   http://localhost:8080")
	fmt.Println("")

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(1 * time.Second)

	// Execute some demo runs
	fmt.Println("🎬 Starting demo executions...")
	fmt.Println("")

	queries := []string{
		"What's the weather in Paris?",
		"What time is it in Tokyo?",
		"Tell me about Berlin's weather and the current time in EST",
	}

	for i, query := range queries {
		fmt.Printf("▶️  Run %d: %s\n", i+1, query)

		runID, err := server.ExecuteGraph(ctx, "demo-agent", query)
		if err != nil {
			log.Printf("   ❌ Failed: %v\n", err)
			continue
		}

		fmt.Printf("   ✅ Started (ID: %s)\n", runID[:12]+"...")
		fmt.Println("")

		time.Sleep(3 * time.Second)
	}

	fmt.Println("✨ Demo runs completed!")
	fmt.Println("")
	fmt.Println("📊 Explore the UI at: http://localhost:8080")
	fmt.Println("   • Click on runs to see details")
	fmt.Println("   • Watch real-time events")
	fmt.Println("   • Check analytics tab for costs")
	fmt.Println("   • View state and checkpoints")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop")

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\n👋 Shutting down...")
	cancel()
}
