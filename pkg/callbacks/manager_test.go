package callbacks

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// testPlugin tracks calls for testing
type testPlugin struct {
	NoopPlugin
	initCalled       bool
	shutdownCalled   bool
	beforeModelCalls int
}

func (p *testPlugin) Init(ctx context.Context) error {
	p.initCalled = true
	return nil
}

func (p *testPlugin) Shutdown(ctx context.Context) error {
	p.shutdownCalled = true
	return nil
}

func (p *testPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	p.beforeModelCalls++
	return nil, nil
}

func TestPluginManager_Registration(t *testing.T) {
	manager := NewPluginManager()
	plugin := &testPlugin{}

	if manager.HasPlugins() {
		t.Fatal("expected no plugins initially")
	}

	err := manager.Register(context.Background(), plugin)
	if err != nil {
		t.Fatalf("failed to register plugin: %v", err)
	}

	if !plugin.initCalled {
		t.Fatal("expected Init to be called on registration")
	}

	if !manager.HasPlugins() {
		t.Fatal("expected HasPlugins to return true after registration")
	}
}

func TestPluginManager_BeforeModel(t *testing.T) {
	manager := NewPluginManager()
	plugin := &testPlugin{}
	manager.Register(context.Background(), plugin)

	req := &model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("test")},
	}

	resp, err := manager.ExecuteBeforeModel(context.Background(), req)
	if err != nil {
		t.Fatalf("ExecuteBeforeModel failed: %v", err)
	}

	if resp != nil {
		t.Fatal("expected no response (no short-circuit)")
	}

	if plugin.beforeModelCalls != 1 {
		t.Fatalf("expected BeforeModel to be called once, got %d", plugin.beforeModelCalls)
	}
}
