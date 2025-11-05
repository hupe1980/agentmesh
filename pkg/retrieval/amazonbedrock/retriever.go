package amazonbedrock

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
)

// Client is an interface representing the Bedrock Agent Runtime client.
type Client interface {
	Retrieve(
		context.Context,
		*bedrockagentruntime.RetrieveInput,
		...func(*bedrockagentruntime.Options),
	) (*bedrockagentruntime.RetrieveOutput, error)
}

// Options defines the configuration options for the retriever.
type Options struct {
	// RetrievalConfiguration provides search parameters for retrieving from knowledge base.
	RetrievalConfiguration types.KnowledgeBaseRetrievalConfiguration
}

// Retriever queries an Amazon Bedrock knowledge base and converts the response
// into AgentMesh retrieval documents.
type Retriever struct {
	client          Client
	knowledgeBaseID string
	opts            Options
}

// NewRetriever creates a new Retriever instance with the given options.
func NewRetriever(client Client, knowledgeBaseID string, optFns ...func(o *Options)) *Retriever {
	opts := Options{
		RetrievalConfiguration: types.KnowledgeBaseRetrievalConfiguration{
			VectorSearchConfiguration: &types.KnowledgeBaseVectorSearchConfiguration{
				NumberOfResults: aws.Int32(3),
			},
		},
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Retriever{
		client:          client,
		knowledgeBaseID: knowledgeBaseID,
		opts:            opts,
	}
}

// Retrieve queries the Amazon Bedrock knowledge base and returns the results as retrieval documents.
func (r *Retriever) Retrieve(ctx context.Context, query string) ([]retrieval.Document, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("empty amazonbedrock query string")
	}

	p := bedrockagentruntime.NewRetrievePaginator(r.client, &bedrockagentruntime.RetrieveInput{
		KnowledgeBaseId: aws.String(r.knowledgeBaseID),
		RetrievalQuery: &types.KnowledgeBaseQuery{
			Text: aws.String(query),
		},
		RetrievalConfiguration: &r.opts.RetrievalConfiguration,
	})

	docs := []retrieval.Document{}

	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, result := range page.RetrievalResults {
			docs = append(docs, retrieval.Document{
				PageContent: aws.ToString(result.Content.Text),
				Score:       aws.ToFloat64(result.Score),
				Metadata: map[string]any{
					"location": aws.ToString(result.Location.S3Location.Uri),
				},
			})
		}
	}

	return docs, nil
}

// Ensure Retriever implements retrieval.Retriever interface
var _ retrieval.Retriever = (*Retriever)(nil)
