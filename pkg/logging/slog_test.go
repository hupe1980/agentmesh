package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memSink stores captured records across handlers.
type memSink struct {
	records []captured
}

type captured struct {
	msg   string
	level slog.Level
	attrs []slog.Attr
}

// memHandler is a minimal slog.Handler for tests that shares a sink and accumulates base attrs.
type memHandler struct {
	sink  *memSink
	attrs []slog.Attr
}

func newMemHandler() *memHandler { return &memHandler{sink: &memSink{}} }

func (h *memHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *memHandler) Handle(_ context.Context, r slog.Record) error {
	// gather attributes from base + record
	final := make([]slog.Attr, 0, len(h.attrs)+8)
	final = append(final, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		final = append(final, a)
		return true
	})
	// store message and level
	h.sink.records = append(h.sink.records, captured{msg: r.Message, level: r.Level, attrs: final})
	return nil
}

func (h *memHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(cp, h.attrs)
	copy(cp[len(h.attrs):], attrs)
	return &memHandler{sink: h.sink, attrs: cp}
}

func (h *memHandler) WithGroup(string) slog.Handler { return &memHandler{sink: h.sink, attrs: h.attrs} }

func TestSlogAdapter_StructuredArgsBecomeAttrs(t *testing.T) {
	mh := newMemHandler()
	logger := slog.New(mh)
	l := NewSlogAdapter(logger)

	l.Info("hello", "k", "x", "n", 7)

	require.Len(t, mh.sink.records, 1)
	assert.Equal(t, "hello", mh.sink.records[0].msg)
	got := map[string]any{}
	for _, a := range mh.sink.records[0].attrs {
		got[a.Key] = a.Value.Any()
	}
	assert.Equal(t, "x", got["k"])
	switch v := got["n"].(type) {
	case int:
		assert.Equal(t, 7, v)
	case int64:
		assert.Equal(t, int64(7), v)
	default:
		t.Fatalf("unexpected type for n: %T", got["n"])
	}
}

func TestSlogAdapter_LevelEmitted(t *testing.T) {
	mh := newMemHandler()
	logger := slog.New(mh)
	l := NewSlogAdapter(logger)

	l.Debug("d")
	l.Info("i")
	l.Warn("w")
	l.Error("e")

	require.Len(t, mh.sink.records, 4)
	levels := []slog.Level{
		mh.sink.records[0].level,
		mh.sink.records[1].level,
		mh.sink.records[2].level,
		mh.sink.records[3].level,
	}
	assert.Equal(t, []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}, levels)
}

func TestSlogAdapter_WithAddsAttributes(t *testing.T) {
	mh := newMemHandler()
	logger := slog.New(mh)
	base := NewSlogAdapter(logger)

	l := base.With("run_id", "r1", "n", 2)
	l.Info("started")

	require.Len(t, mh.sink.records, 1)
	// collect attrs into a map for easy assertions
	got := map[string]any{}
	for _, a := range mh.sink.records[0].attrs {
		got[a.Key] = a.Value.Any()
	}
	assert.Equal(t, "r1", got["run_id"])
	// slog normalizes numeric values, accept int64
	switch v := got["n"].(type) {
	case int:
		assert.Equal(t, 2, v)
	case int64:
		assert.Equal(t, int64(2), v)
	default:
		t.Fatalf("unexpected type for n: %T", got["n"])
	}
}
