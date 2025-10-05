package amazonkendra

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kendra"
	"github.com/aws/aws-sdk-go-v2/service/kendra/types"
	"github.com/hupe1980/agentmesh/tool/retrieval"
)

// Client represents a client for interacting with Amazon Kendra.
type Client interface {
	// Retrieve retrieves documents from Amazon Kendra based on the provided input parameters.
	// It returns the retrieval output or an error if the retrieval operation fails.
	Retrieve(
		ctx context.Context,
		params *kendra.RetrieveInput,
		optFns ...func(*kendra.Options),
	) (*kendra.RetrieveOutput, error)
}

// Options defines the configuration options for the retriever.
type Options struct {
	// Number of documents to query for
	TopK int32

	// AttributeFilter provides filtering the results based on document attributes or metadata
	// fields.
	AttributeFilter *types.AttributeFilter

	// UserContext provides information about the user context for an Amazon Kendra index.
	UserContext *types.UserContext
}

// Retriever wraps an Amazon Kendra client and exposes the AgentMesh retrieval
// interface for querying an index and mapping results into documents.
type Retriever struct {
	client Client
	index  string
	opts   Options
}

// NewRetriever creates a new Retriever instance with the specified options.
func NewRetriever(client Client, index string, optFns ...func(o *Options)) *Retriever {
	opts := Options{
		TopK: 3,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Retriever{
		client: client,
		index:  index,
		opts:   opts,
	}
}

// Retrieve retrieves documents from Amazon Kendra based on the provided query.
func (r *Retriever) Retrieve(ctx context.Context, query string) ([]retrieval.Document, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("empty amazonkendra query string")
	}

	output, err := r.client.Retrieve(ctx, &kendra.RetrieveInput{
		IndexId:         aws.String(r.index),
		QueryText:       aws.String(query),
		PageSize:        aws.Int32(r.opts.TopK),
		AttributeFilter: r.opts.AttributeFilter,
		UserContext:     r.opts.UserContext,
	})
	if err != nil {
		return nil, err
	}

	if output == nil || len(output.ResultItems) == 0 {
		return []retrieval.Document{}, nil
	}

	docs := make([]retrieval.Document, 0, len(output.ResultItems))
	for _, item := range output.ResultItems {
		docs = append(docs, parseRetrievalResultItem(item))
	}

	return docs, nil
}

// parseRetrievalResultItem parses a single retrieval result item into a Document.
func parseRetrievalResultItem(item types.RetrieveResultItem) retrieval.Document {
	title := aws.ToString(item.DocumentTitle)
	content := cleanText(aws.ToString(item.Content))
	source := aws.ToString(item.DocumentURI)

	var score float64
	if item.ScoreAttributes != nil {
		score = mapScoreConfidenceToFloat(item.ScoreAttributes.ScoreConfidence)
	} else {
		score = 0.0
	}

	return retrieval.Document{
		PageContent: content,
		Score:       score,
		Metadata: map[string]any{
			"source": source,
			"title":  title,
		},
	}
}

// whitespaceRe is a regular expression used for matching whitespace characters.
var whitespaceRe = regexp.MustCompile(`\s+`)

// cleanText removes excess whitespace and ellipsis from the given string.
func cleanText(resText string) string {
	if resText == "" {
		return ""
	}

	cleanedText := whitespaceRe.ReplaceAllString(resText, " ")
	cleanedText = strings.ReplaceAll(cleanedText, "...", "")

	return cleanedText
}

// mapScoreConfidenceToFloat maps a ScoreConfidence enum to a float64 value.
func mapScoreConfidenceToFloat(conf types.ScoreConfidence) float64 {
	switch conf {
	case types.ScoreConfidenceVeryHigh:
		return 1.0
	case types.ScoreConfidenceHigh:
		return 0.75
	case types.ScoreConfidenceMedium:
		return 0.5
	case types.ScoreConfidenceLow:
		return 0.25
	default:
		return 0.0 // NOT_AVAILABLE or missing
	}
}

var _ retrieval.Retriever = (*Retriever)(nil)
