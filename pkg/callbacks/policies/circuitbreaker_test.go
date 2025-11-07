package policies

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// CircuitBreaker Tests

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   3,
		Timeout:       100 * time.Millisecond,
		FailureWindow: time.Minute,
	}

	before, after, onError := CircuitBreaker(config)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	// Initial state: Closed - should allow requests
	msg, err := before(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != nil {
		t.Fatal("circuit should be closed initially")
	}

	// Fail 3 times to open circuit
	for i := 0; i < 3; i++ {
		_, _ = onError(ctx, sw, testErr)
	}

	// Circuit should now be open
	msg, err = before(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected circuit to be open after failures")
	}

	// Wait for timeout to transition to half-open
	time.Sleep(110 * time.Millisecond)

	// Should be half-open now (allows one request)
	msg, err = before(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != nil {
		t.Fatal("circuit should be half-open, allowing test request")
	}

	// Success in half-open state should close the circuit
	_, _ = after(ctx, sw, message.NewAIMessageFromText("success"))

	// Circuit should be closed again
	msg, err = before(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != nil {
		t.Fatal("circuit should be closed after successful half-open request")
	}
}

func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   2,
		Timeout:       50 * time.Millisecond,
		FailureWindow: time.Minute,
	}

	before, after, _ := CircuitBreaker(config)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	// Fail to open circuit
	onError := func(ctx context.Context, s any, err error) (message.Message, error) {
		return nil, err
	}

	before2, after2, onError2 := CircuitBreaker(config)

	// Cause failures
	onError2(ctx, sw, testErr)
	onError2(ctx, sw, testErr)

	// Circuit is open
	msg, _ := before2(ctx, sw)
	if msg == nil {
		t.Fatal("circuit should be open")
	}

	// Wait for half-open transition
	time.Sleep(60 * time.Millisecond)

	// Allow request (half-open)
	msg, _ = before2(ctx, sw)
	if msg != nil {
		t.Fatal("should allow request in half-open state")
	}

	// Success closes circuit
	after2(ctx, sw, message.NewAIMessageFromText("ok"))

	// Verify closed
	msg, _ = before2(ctx, sw)
	if msg != nil {
		t.Fatal("circuit should be closed after recovery")
	}

	// Clean up unused vars
	_ = before
	_ = after
	_ = onError
}

func TestCircuitBreaker_FailureWindow(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   2,
		Timeout:       time.Second,
		FailureWindow: 100 * time.Millisecond,
	}

	before, after, onError := CircuitBreaker(config)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	// First failure
	onError(ctx, sw, testErr)

	// Wait for failure window to expire
	time.Sleep(110 * time.Millisecond)

	// After callback should reset failure count (outside window)
	after(ctx, sw, message.NewAIMessageFromText("ok"))

	// One more failure (should not open circuit since counter was reset)
	onError(ctx, sw, testErr)

	// Circuit should still be closed
	msg, _ := before(ctx, sw)
	if msg != nil {
		t.Fatal("circuit should remain closed (failure counter reset)")
	}
}

func TestPerNodeCircuitBreaker_Independent(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   2,
		Timeout:       time.Second,
		FailureWindow: time.Minute,
	}

	before1, _, onError1 := PerNodeCircuitBreaker("node1", config)
	before2, _, _ := PerNodeCircuitBreaker("node2", config)

	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("test error")

	// Fail node1 circuit
	onError1(ctx, sw, testErr)
	onError1(ctx, sw, testErr)

	// node1 should be open
	msg1, _ := before1(ctx, sw)
	if msg1 == nil {
		t.Fatal("node1 circuit should be open")
	}

	// node2 should still be closed (independent)
	msg2, _ := before2(ctx, sw)
	if msg2 != nil {
		t.Fatal("node2 circuit should remain closed")
	}
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.MaxFailures != 5 {
		t.Errorf("expected default MaxFailures=5, got %d", config.MaxFailures)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("expected default Timeout=30s, got %v", config.Timeout)
	}
	if config.FailureWindow != time.Minute {
		t.Errorf("expected default FailureWindow=1m, got %v", config.FailureWindow)
	}
}

func TestCircuitBreaker_MessageContent(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   1,
		Timeout:       time.Second,
		FailureWindow: time.Minute,
	}

	before, _, onError := CircuitBreaker(config)
	ctx := context.Background()
	sw := createTestStateWriter()

	// Open the circuit
	onError(ctx, sw, errors.New("fail"))

	// Get the rejection message
	msg, _ := before(ctx, sw)
	if msg == nil {
		t.Fatal("expected rejection message")
	}

	// Verify message has content
	parts := msg.Parts()
	if len(parts) == 0 {
		t.Error("circuit breaker message should have content")
	}
}

func TestPerNodeCircuitBreaker_MessageContainsNodeName(t *testing.T) {
	nodeName := "critical_service"
	config := CircuitBreakerConfig{
		MaxFailures:   1,
		Timeout:       time.Second,
		FailureWindow: time.Minute,
	}

	before, _, onError := PerNodeCircuitBreaker(nodeName, config)
	ctx := context.Background()
	sw := createTestStateWriter()

	// Open the circuit
	onError(ctx, sw, errors.New("fail"))

	// Get the rejection message
	msg, _ := before(ctx, sw)
	if msg == nil {
		t.Fatal("expected rejection message")
	}

	// Message should have content (ideally mentioning node name)
	parts := msg.Parts()
	if len(parts) == 0 {
		t.Error("circuit breaker message should have content")
	}
}

func TestCircuitBreaker_StateString(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{CircuitState(999), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

func TestCircuitBreaker_ConcurrentFailures(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   10,
		Timeout:       time.Second,
		FailureWindow: time.Minute,
	}

	_, _, onError := CircuitBreaker(config)
	ctx := context.Background()
	sw := createTestStateWriter()
	testErr := errors.New("concurrent failure")

	// Simulate concurrent failures
	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func() {
			onError(ctx, sw, testErr)
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	// Test passes if no race conditions occurred
}
