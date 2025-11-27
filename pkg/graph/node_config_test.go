package graph_test

import (
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRetryPolicy(t *testing.T) {
	policy := &graph.RetryPolicy{
		MaxAttempts: 5,
		Backoff:     graph.ExponentialBackoff(100*time.Millisecond, 2.0),
	}

	config := &graph.NodeConfig{}
	opt := graph.WithRetryPolicy(policy)
	opt(config)

	assert.Equal(t, policy, config.RetryPolicy)
	assert.Equal(t, 5, policy.MaxAttempts)
}

func TestWithRetryPolicy_NilPolicy(t *testing.T) {
	config := &graph.NodeConfig{}
	opt := graph.WithRetryPolicy(nil)
	opt(config)

	assert.Nil(t, config.RetryPolicy)
}

func TestNodeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *graph.NodeConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid_config_with_retry",
			config: &graph.NodeConfig{
				RetryPolicy: &graph.RetryPolicy{
					MaxAttempts: 3,
					Backoff:     graph.ConstantBackoff(100),
				},
			},
			wantErr: false,
		},
		{
			name: "valid_config_no_retry",
			config: &graph.NodeConfig{
				RetryPolicy: nil,
			},
			wantErr: false,
		},
		{
			name: "valid_config_single_attempt",
			config: &graph.NodeConfig{
				RetryPolicy: &graph.RetryPolicy{
					MaxAttempts: 1,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_zero_attempts",
			config: &graph.NodeConfig{
				RetryPolicy: &graph.RetryPolicy{
					MaxAttempts: 0,
				},
			},
			wantErr: true,
			errMsg:  "retry policy: MaxAttempts must be >= 1",
		},
		{
			name: "invalid_negative_attempts",
			config: &graph.NodeConfig{
				RetryPolicy: &graph.RetryPolicy{
					MaxAttempts: -1,
				},
			},
			wantErr: true,
			errMsg:  "retry policy: MaxAttempts must be >= 1",
		},
		{
			name: "valid_large_attempts",
			config: &graph.NodeConfig{
				RetryPolicy: &graph.RetryPolicy{
					MaxAttempts: 1000,
					Backoff:     graph.ExponentialBackoff(50*time.Millisecond, 2.0),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNodeConfig_MultipleOptions(t *testing.T) {
	// Test that multiple options can be applied
	config := &graph.NodeConfig{}

	policy := &graph.RetryPolicy{
		MaxAttempts: 3,
		Backoff:     graph.LinearBackoff(50),
	}

	opt := graph.WithRetryPolicy(policy)
	opt(config)

	assert.NotNil(t, config.RetryPolicy)
	assert.Equal(t, 3, config.RetryPolicy.MaxAttempts)

	err := config.Validate()
	require.NoError(t, err)
}

func TestNodeOption_ChainApply(t *testing.T) {
	// Test that options can be chained
	policy1 := &graph.RetryPolicy{
		MaxAttempts: 2,
		Backoff:     graph.ConstantBackoff(100),
	}

	policy2 := &graph.RetryPolicy{
		MaxAttempts: 5,
		Backoff:     graph.ExponentialBackoff(200*time.Millisecond, 2.0),
	}

	config := &graph.NodeConfig{}

	// Apply first option
	opt1 := graph.WithRetryPolicy(policy1)
	opt1(config)
	assert.Equal(t, 2, config.RetryPolicy.MaxAttempts)

	// Apply second option (should override)
	opt2 := graph.WithRetryPolicy(policy2)
	opt2(config)
	assert.Equal(t, 5, config.RetryPolicy.MaxAttempts)
}

func TestNodeConfig_EmptyConfig(t *testing.T) {
	// Empty config should be valid
	config := &graph.NodeConfig{}
	err := config.Validate()
	require.NoError(t, err)
}
