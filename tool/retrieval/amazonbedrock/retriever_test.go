package amazonbedrock

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"
	"github.com/hupe1980/agentmesh/tool/retrieval"
	"github.com/stretchr/testify/require"
)

type mockBedrockAgentRuntimeClient struct {
	outputs []*bedrockagentruntime.RetrieveOutput
	err     error
	calls   []*bedrockagentruntime.RetrieveInput
}

func (m *mockBedrockAgentRuntimeClient) Retrieve(
	ctx context.Context,
	input *bedrockagentruntime.RetrieveInput,
	optFns ...func(*bedrockagentruntime.Options),
) (*bedrockagentruntime.RetrieveOutput, error) {
	m.calls = append(m.calls, input)

	if m.err != nil {
		return nil, m.err
	}

	if len(m.outputs) == 0 {
		return &bedrockagentruntime.RetrieveOutput{}, nil
	}

	output := m.outputs[0]
	m.outputs = m.outputs[1:]

	return output, nil
}

func TestRetriever_Retrieve(t *testing.T) {
	retrieveErr := errors.New("retrieve error")

	tests := []struct {
		name          string
		client        *mockBedrockAgentRuntimeClient
		query         string
		expectedDocs  []retrieval.Document
		expectedErr   error
		expectKBID    string
		expectTrimmed string
	}{
		{
			name: "retrieve success",
			client: &mockBedrockAgentRuntimeClient{
				outputs: []*bedrockagentruntime.RetrieveOutput{
					{
						RetrievalResults: []types.KnowledgeBaseRetrievalResult{
							{
								Content: &types.RetrievalResultContent{
									Text: aws.String("Content 1"),
								},
								Location: &types.RetrievalResultLocation{
									Type: types.RetrievalResultLocationTypeS3,
									S3Location: &types.RetrievalResultS3Location{
										Uri: aws.String("URI 1"),
									},
								},
								Score: aws.Float64(0.9),
							},
							{
								Content: &types.RetrievalResultContent{
									Text: aws.String("Content 2"),
								},
								Location: &types.RetrievalResultLocation{
									Type: types.RetrievalResultLocationTypeS3,
									S3Location: &types.RetrievalResultS3Location{
										Uri: aws.String("URI 2"),
									},
								},
								Score: aws.Float64(0.8),
							},
						},
					},
				},
			},
			query: "  query  ",
			expectedDocs: []retrieval.Document{
				{
					PageContent: "Content 1",
					Score:       0.9,
					Metadata: map[string]any{
						"location": "URI 1",
					},
				},
				{
					PageContent: "Content 2",
					Score:       0.8,
					Metadata: map[string]any{
						"location": "URI 2",
					},
				},
			},
			expectedErr:   nil,
			expectKBID:    "knowledge-base-id",
			expectTrimmed: "query",
		},
		{
			name: "retrieve error",
			client: &mockBedrockAgentRuntimeClient{
				err: retrieveErr,
			},
			query:        "query",
			expectedDocs: nil,
			expectedErr:  retrieveErr,
			expectKBID:   "knowledge-base-id",
		},
		{
			name: "no results",
			client: &mockBedrockAgentRuntimeClient{
				outputs: []*bedrockagentruntime.RetrieveOutput{
					{
						RetrievalResults: []types.KnowledgeBaseRetrievalResult{},
					},
				},
			},
			query:        "query",
			expectedDocs: []retrieval.Document{},
			expectedErr:  nil,
			expectKBID:   "knowledge-base-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRetriever(tt.client, "knowledge-base-id")

			docs, err := r.Retrieve(context.Background(), tt.query)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				require.Nil(t, docs)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedDocs, docs)
			}

			require.NotEmpty(t, tt.client.calls)
			firstCall := tt.client.calls[0]
			require.NotNil(t, firstCall)
			require.NotNil(t, firstCall.KnowledgeBaseId)
			require.Equal(t, tt.expectKBID, aws.ToString(firstCall.KnowledgeBaseId))

			if tt.expectTrimmed != "" {
				require.NotNil(t, firstCall.RetrievalQuery)
				require.NotNil(t, firstCall.RetrievalQuery.Text)
				require.Equal(t, tt.expectTrimmed, aws.ToString(firstCall.RetrievalQuery.Text))
			}
		})
	}
}
