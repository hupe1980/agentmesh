package retrieval

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/tool"
)

// Document represents a single piece of retrieved content and its metadata.
type Document struct {
	PageContent string         `json:"page_content"`
	Score       float64        `json:"score"`
	Metadata    map[string]any `json:"metadata"`
}

// Retriever defines the contract for retrieving documents for a given query.
type Retriever interface {
	Retrieve(ctx context.Context, query string) ([]Document, error)
}

// NewTool wraps a retriever into a callable tool that accepts a query argument.
func NewTool(name, description string, retriever Retriever) core.Tool {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]string{
				"title": "The query to retrieve.",
				"type":  "string",
			},
		},
		"required": []string{"query"},
	}

	return tool.NewFuncTool(
		name,
		description,
		params,
		func(
			ctx context.Context,
			tc core.ToolContext,
			args map[string]any,
		) (any, error) {
			query, ok := args["query"].(string)
			if !ok {
				return nil, tool.NewError(name, "missing required field 'query'", "VALIDATION_ERROR")
			}

			return retriever.Retrieve(ctx, query)
		},
	)
}
