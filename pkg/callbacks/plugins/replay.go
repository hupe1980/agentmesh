package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// ReplayMode determines whether the plugin is recording or replaying.
type ReplayMode string

const (
	// RecordMode records all model responses
	RecordMode ReplayMode = "record"
	// ModeReplay replays recorded responses
	ModeReplay ReplayMode = "replay"
)

// ReplayPlugin records and replays model responses for deterministic testing.
// In record mode, it stores all responses. In replay mode, it returns stored responses.
type ReplayPlugin struct {
	callbacks.NoopPlugin

	mode       ReplayMode
	mu         sync.RWMutex
	recordings map[string]*model.Response
}

// NewReplayPlugin creates a replay plugin in the specified mode.
func NewReplayPlugin(mode ReplayMode) *ReplayPlugin {
	return &ReplayPlugin{
		mode:       mode,
		recordings: make(map[string]*model.Response),
	}
}

// BeforeModel returns recorded responses in replay mode.
func (p *ReplayPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	if p.mode != ModeReplay {
		return nil, nil
	}

	// Generate key from request
	key := p.requestKey(req)

	p.mu.RLock()
	recorded, ok := p.recordings[key]
	p.mu.RUnlock()

	if ok {
		// Return recorded response (short-circuit)
		return recorded, nil
	}

	// No recording found
	return nil, fmt.Errorf("no recording found for request (key: %s)", key)
}

// AfterModel records model responses in record mode.
func (p *ReplayPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	if p.mode != RecordMode {
		return nil, nil
	}

	// Record the response
	key := p.requestKey(req)

	p.mu.Lock()
	p.recordings[key] = resp
	p.mu.Unlock()

	return nil, nil
}

// requestKey generates a unique key for a request.
func (p *ReplayPlugin) requestKey(req *model.Request) string {
	h := sha256.New()

	// Hash messages
	for _, msg := range req.Messages {
		h.Write([]byte(msg.Type()))
		h.Write([]byte(message.Stringify(msg)))
	}

	// Hash system prompt
	if req.SystemPrompt != "" {
		h.Write([]byte(req.SystemPrompt))
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// SaveRecordings saves recordings to a writer in JSON format.
func (p *ReplayPlugin) SaveRecordings(w io.Writer) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(p.recordings); err != nil {
		return fmt.Errorf("failed to encode recordings: %w", err)
	}

	return nil
}

// LoadRecordings loads recordings from a reader in JSON format.
func (p *ReplayPlugin) LoadRecordings(r io.Reader) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	decoder := json.NewDecoder(r)

	if err := decoder.Decode(&p.recordings); err != nil {
		return fmt.Errorf("failed to decode recordings: %w", err)
	}

	return nil
}

// GetRecordingCount returns the number of stored recordings.
func (p *ReplayPlugin) GetRecordingCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.recordings)
}

// Clear removes all recordings.
func (p *ReplayPlugin) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recordings = make(map[string]*model.Response)
}
