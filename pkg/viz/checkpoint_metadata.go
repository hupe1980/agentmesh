package viz

import (
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// EnhancedCheckpointMetadata provides rich metadata for checkpoint visualization
type EnhancedCheckpointMetadata struct {
	// Basic info
	RunID     string    `json:"run_id"`
	Superstep int64     `json:"superstep"`
	Version   uint64    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Committed bool      `json:"committed"`

	// Execution metrics
	Duration       time.Duration `json:"duration,omitempty"`        // Time since last checkpoint
	TotalDuration  time.Duration `json:"total_duration,omitempty"`  // Total execution time
	MemoryUsageKB  uint64        `json:"memory_usage_kb,omitempty"` // Memory usage at checkpoint
	NodeCount      int           `json:"node_count"`                // Total nodes in graph
	CompletedCount int           `json:"completed_count"`           // Completed nodes count
	PausedCount    int           `json:"paused_count"`              // Paused nodes count
	PendingCount   int           `json:"pending_count"`             // Pending writes count

	// Node states
	CompletedNodes []string `json:"completed_nodes"`
	PausedNodes    []string `json:"paused_nodes,omitempty"`
	ActiveNodes    []string `json:"active_nodes,omitempty"` // Nodes executing at checkpoint time

	// State information
	StateKeys     []string       `json:"state_keys"`              // All state channel keys
	StateSize     int            `json:"state_size,omitempty"`    // Approximate state size in bytes
	MessageCount  int            `json:"message_count,omitempty"` // Number of messages in state
	PendingWrites []PendingWrite `json:"pending_writes,omitempty"`

	// Token usage (if available in metadata)
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	TotalTokens  int     `json:"total_tokens,omitempty"`
	EstCostUSD   float64 `json:"est_cost_usd,omitempty"`

	// Navigation
	HasPrevious bool  `json:"has_previous"` // Is there a previous checkpoint?
	HasNext     bool  `json:"has_next"`     // Is there a next checkpoint?
	PrevStep    int64 `json:"prev_step,omitempty"`
	NextStep    int64 `json:"next_step,omitempty"`

	// Custom metadata from checkpoint
	Metadata map[string]any `json:"metadata,omitempty"`
}

// PendingWrite represents a pending state write (from checkpoint package)
type PendingWrite struct {
	NodeName  string    `json:"node_name"`
	Channel   string    `json:"channel"`
	Value     any       `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

// ConvertCheckpointToMetadata converts a checkpoint to enhanced metadata
func ConvertCheckpointToMetadata(cp *checkpoint.Checkpoint, allCheckpoints []*checkpoint.Checkpoint) EnhancedCheckpointMetadata {
	metadata := EnhancedCheckpointMetadata{
		RunID:          cp.RunID,
		Superstep:      cp.Superstep,
		Version:        cp.Version,
		Timestamp:      cp.Timestamp,
		Committed:      cp.Committed,
		CompletedCount: len(cp.CompletedNodes),
		PausedCount:    len(cp.PausedNodes),
		PendingCount:   len(cp.PendingWrites),
		CompletedNodes: cp.CompletedNodes,
		PausedNodes:    cp.PausedNodes,
		Metadata:       cp.Metadata,
	}

	// Extract state keys
	metadata.StateKeys = make([]string, 0, len(cp.State))
	for key := range cp.State {
		metadata.StateKeys = append(metadata.StateKeys, key)
	}

	// Convert pending writes
	metadata.PendingWrites = make([]PendingWrite, len(cp.PendingWrites))
	for i, pw := range cp.PendingWrites {
		metadata.PendingWrites[i] = PendingWrite{
			NodeName:  pw.NodeName,
			Channel:   pw.Channel,
			Value:     pw.Value,
			Timestamp: pw.Timestamp,
		}
	}

	// Extract token usage from metadata if available
	extractCheckpointMetadata(&metadata, cp.Metadata)

	// Calculate navigation info
	if len(allCheckpoints) > 0 {
		currentIdx := -1
		for i, c := range allCheckpoints {
			if c.Superstep == cp.Superstep {
				currentIdx = i
				break
			}
		}

		if currentIdx > 0 {
			metadata.HasPrevious = true
			metadata.PrevStep = allCheckpoints[currentIdx-1].Superstep
		}

		if currentIdx >= 0 && currentIdx < len(allCheckpoints)-1 {
			metadata.HasNext = true
			metadata.NextStep = allCheckpoints[currentIdx+1].Superstep
		}
	}

	return metadata
}

// CheckpointDiffResponse represents the difference between two checkpoints
type CheckpointDiffResponse struct {
	FromSuperstep int64       `json:"from_superstep"`
	ToSuperstep   int64       `json:"to_superstep"`
	StateDiffs    []StateDiff `json:"state_diffs"`
	Summary       DiffSummary `json:"summary"`
}

// DiffSummary provides a high-level summary of changes
type DiffSummary struct {
	AddedKeys     int      `json:"added_keys"`
	RemovedKeys   int      `json:"removed_keys"`
	ModifiedKeys  int      `json:"modified_keys"`
	NodesAdded    []string `json:"nodes_added,omitempty"`
	NodesRemoved  []string `json:"nodes_removed,omitempty"`
	WritesApplied int      `json:"writes_applied"`
}

// extractCheckpointMetadata extracts metadata fields from checkpoint metadata map
func extractCheckpointMetadata(metadata *EnhancedCheckpointMetadata, cpMetadata map[string]any) {
	if cpMetadata == nil {
		return
	}

	if inputTokens, ok := cpMetadata["input_tokens"].(int); ok {
		metadata.InputTokens = inputTokens
	}
	if outputTokens, ok := cpMetadata["output_tokens"].(int); ok {
		metadata.OutputTokens = outputTokens
	}
	if totalTokens, ok := cpMetadata["total_tokens"].(int); ok {
		metadata.TotalTokens = totalTokens
	}
	if cost, ok := cpMetadata["cost_usd"].(float64); ok {
		metadata.EstCostUSD = cost
	}
	if duration, ok := cpMetadata["duration"].(time.Duration); ok {
		metadata.Duration = duration
	}
	if totalDuration, ok := cpMetadata["total_duration"].(time.Duration); ok {
		metadata.TotalDuration = totalDuration
	}
	if memory, ok := cpMetadata["memory_kb"].(uint64); ok {
		metadata.MemoryUsageKB = memory
	}
}
