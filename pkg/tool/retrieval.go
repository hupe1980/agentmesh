package tool

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/retrieval"
)

// RetrievalArgs defines the arguments for the retrieval tool.
type RetrievalArgs struct {
	Query string `json:"query" jsonschema:"title=The query to retrieve.,required"`
}

// NewRetrievalTool creates a new retrieval tool that queries the given retriever.
func NewRetrievalTool(name, description string, retriever retrieval.Retriever) (*FuncTool[RetrievalArgs, []retrieval.Document], error) {
	return NewFuncTool(
		name,
		description,
		func(ctx context.Context, args RetrievalArgs) ([]retrieval.Document, error) {
			return retriever.Retrieve(ctx, args.Query)
		},
	)
}
