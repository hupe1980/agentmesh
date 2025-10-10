package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	am "github.com/hupe1980/agentmesh"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model/openai"
	"github.com/hupe1980/agentmesh/tool"
)

// Build a get_weather function tool (mock data; replace with real API calls as needed).
func newGetWeatherTool() core.Tool {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"location": map[string]any{
				"type":        "string",
				"description": "City and country, e.g. 'Berlin, DE'",
			},
		},
		"required": []string{"location"},
	}

	fn := func(ctx context.Context, _ core.ToolContext, args map[string]any) (any, error) {
		loc, _ := args["location"].(string)
		if loc == "" {
			return nil, fmt.Errorf("location is required")
		}

		// Mock response
		return map[string]any{
			"location":      loc,
			"temperature_c": 21.5,
			"condition":     "Partly Cloudy",
			"humidity":      60,
			"wind_kph":      12.3,
		}, nil
	}

	return tool.NewFuncTool("get_weather", "Get current weather information for a location", schema, fn)
}

func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	model := openai.NewModel()

	agent, err := am.NewModelAgent("WeatherAgent", model, func(o *am.ModelAgentOptions) {
		o.Instructions = core.NewInstructionsFromText("You are a weather assistant.")
		o.Tools = []core.Tool{newGetWeatherTool()}
		o.OutputSchema = core.MustNewOutputSchema("weather_response", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location":      map[string]any{"type": "string"},
				"temperature_c": map[string]any{"type": "number"},
				"condition":     map[string]any{"type": "string"},
				"humidity":      map[string]any{"type": "number"},
				"wind_kph":      map[string]any{"type": "number"},
			},
			"required": []string{"location", "temperature_c", "condition", "humidity", "wind_kph"},
		})
	})
	if err != nil {
		log.Fatalf("failed creating agent: %v", err)
	}

	application := am.NewApp("weather_app", agent)

	r := am.NewRunner(application, func(o *am.RunnerOptions) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() {
		_ = r.Close()
	}()

	userParts := []core.Part{core.NewPartFromText("What's the weather like in Berlin?")}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	runID, results, err := r.Run(ctx, "user1", "sess1", userParts)
	if err != nil {
		log.Fatalf("run failed: %v", err)
	}

	fmt.Printf("=== Weather Agent (runID=%s) ===\n", runID)

	accumulate(results, agent.Name())
}

func accumulate(results <-chan core.RunResult, focus string) {
	var answer string
	for res := range results {
		if res.Err != nil {
			log.Printf("error: %v", res.Err)
			continue
		}

		ev := res.Event
		if ev == nil {
			continue
		}

		if ev.Author == focus && len(ev.Parts) > 0 {
			for _, p := range ev.Parts {
				if tp, ok := p.(*core.TextPart); ok {
					answer += tp.Text
				}
			}
		}

		if ev.TurnComplete.Or(false) {
			fmt.Printf("Weather: %s\n", answer)
			return
		}
	}

	if answer != "" {
		fmt.Printf("Weather: %s\n", answer)
	} else {
		fmt.Println("No response")
	}
}
