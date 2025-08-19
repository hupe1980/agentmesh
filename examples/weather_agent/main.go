package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hupe1980/agentmesh"
	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model/openai"
)

// GetWeatherTool returns mock weather data (replace with real API integration).
type GetWeatherTool struct{}

func (t *GetWeatherTool) Name() string { return "get_weather" }

func (t *GetWeatherTool) Description() string {
	return "Get current weather information for a location"
}

func (t *GetWeatherTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"location": map[string]any{
				"type":        "string",
				"description": "City and country, e.g. 'Berlin, DE'",
			},
		},
		"required": []string{"location"},
	}
}

func (t *GetWeatherTool) Call(_ *core.ToolContext, args map[string]any) (any, error) {
	loc, _ := args["location"].(string)
	if loc == "" {
		return nil, fmt.Errorf("location is required")
	}

	return map[string]any{
		"location":      loc,
		"temperature_c": 21.5,
		"condition":     "Partly Cloudy",
		"humidity":      60,
		"wind_kph":      12.3,
	}, nil
}

// weather_agent demonstrates a tool-enabled agent producing a direct answer after a single tool call.
func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	model := openai.NewModel()

	agent := agent.NewModelAgent("WeatherAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instruction = agent.NewInstructionFromText("You are a weather assistant.")
	})
	agent.RegisterTool(&GetWeatherTool{})

	mesh := agentmesh.New(agent, func(o *agentmesh.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, "text", false)
	})

	userContent := newUserText("What's the weather like in Berlin?")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	_, eventsCh, errorsCh, err := mesh.Invoke(ctx, "sess1", userContent)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Println("=== Weather Agent ===")
	accumulate(eventsCh, errorsCh, agent.Name())
}

func accumulate(eventsCh <-chan core.Event, errorsCh <-chan error, focus string) {
	var answer string
	for eventsCh != nil || errorsCh != nil {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				continue
			}
			if ev.Author == focus && ev.Content != nil {
				for _, p := range ev.Content.Parts {
					if tp, ok := p.(core.TextPart); ok {
						answer += tp.Text
					}
				}
			}
			if ev.TurnComplete != nil && *ev.TurnComplete {
				fmt.Printf("Weather: %s\n", answer)
				return
			}
		case err, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				continue
			}
			if err != nil {
				log.Printf("error: %v", err)
			}
		}
	}
	if answer != "" {
		fmt.Printf("Weather: %s\n", answer)
	} else {
		fmt.Println("No response")
	}
}

// Shared helpers
func newUserText(txt string) core.Content {
	return core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: txt}}}
}
