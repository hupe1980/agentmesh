package checkpoint

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPendingWritesStructure verifies the PendingWrite structure
func TestPendingWritesStructure(t *testing.T) {
	now := time.Now()
	chkpt := &Checkpoint{
		RunID:     "test-run-001",
		Superstep: 1,
		Timestamp: now,
		State: map[string]any{
			"channel1": "value1",
		},
		PendingWrites: []PendingWrite{
			{
				NodeName:  "node1",
				Channel:   "output",
				Value:     "pending_value",
				Timestamp: now,
			},
		},
	}

	assert.Len(t, chkpt.PendingWrites, 1)
	assert.Equal(t, "node1", chkpt.PendingWrites[0].NodeName)
	assert.Equal(t, "output", chkpt.PendingWrites[0].Channel)
	assert.Equal(t, "pending_value", chkpt.PendingWrites[0].Value)
	assert.Equal(t, now, chkpt.PendingWrites[0].Timestamp)
}
