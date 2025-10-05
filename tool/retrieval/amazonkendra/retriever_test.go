package amazonkendra

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kendra"
	"github.com/aws/aws-sdk-go-v2/service/kendra/types"
	"github.com/hupe1980/agentmesh/tool/retrieval"
	"github.com/stretchr/testify/require"
)

type mockAmazonKendraClient struct {
	output *kendra.RetrieveOutput
	err    error
	calls  []*kendra.RetrieveInput
}

func (m *mockAmazonKendraClient) Retrieve(
	ctx context.Context,
	params *kendra.RetrieveInput,
	optFns ...func(*kendra.Options),
) (*kendra.RetrieveOutput, error) {
	m.calls = append(m.calls, params)
	return m.output, m.err
}

func TestRetriever_Retrieve(t *testing.T) {
	retrieveErr := errors.New("retrieve error")

	tests := []struct {
		name         string
		client       *mockAmazonKendraClient
		query        string
		expectedDocs []retrieval.Document
		expectedErr  error
	}{
		{
			name: "retrieve success",
			client: &mockAmazonKendraClient{
				output: &kendra.RetrieveOutput{
					ResultItems: []types.RetrieveResultItem{
						{
							DocumentTitle: aws.String("Title 1"),
							Content:       aws.String("Content 1"),
							DocumentURI:   aws.String("URI 1"),
						},
						{
							DocumentTitle: aws.String("Title 2"),
							Content:       aws.String("Content 2"),
							DocumentURI:   aws.String("URI 2"),
						},
					},
				},
			},
			query: " query ",
			expectedDocs: []retrieval.Document{
				{
					PageContent: "Content 1",
					Metadata: map[string]any{
						"source": "URI 1",
						"title":  "Title 1",
					},
				},
				{
					PageContent: "Content 2",
					Metadata: map[string]any{
						"source": "URI 2",
						"title":  "Title 2",
					},
				},
			},
		},
		{
			name: "retrieve error",
			client: &mockAmazonKendraClient{
				err: retrieveErr,
			},
			query:        "query",
			expectedDocs: nil,
			expectedErr:  retrieveErr,
		},
		{
			name: "no results",
			client: &mockAmazonKendraClient{
				output: &kendra.RetrieveOutput{
					ResultItems: []types.RetrieveResultItem{},
				},
			},
			query:        "query",
			expectedDocs: []retrieval.Document{},
		},
		{
			name:         "empty query",
			client:       &mockAmazonKendraClient{},
			query:        "   ",
			expectedDocs: nil,
			expectedErr:  errors.New("empty amazonkendra query string"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRetriever(tt.client, "index-id")

			docs, err := r.Retrieve(context.Background(), tt.query)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.EqualError(t, err, tt.expectedErr.Error())
				require.Nil(t, docs)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedDocs, docs)
			}

			trimmed := strings.TrimSpace(tt.query)
			if trimmed == "" {
				require.Empty(t, tt.client.calls)
				return
			}

			require.NotEmpty(t, tt.client.calls)
			call := tt.client.calls[0]
			require.Equal(t, "index-id", aws.ToString(call.IndexId))
			require.Equal(t, trimmed, aws.ToString(call.QueryText))
		})
	}
}
