package artifact

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contentString extracts a comparable string from a *core.FilePart's content.
func contentString(t *testing.T, p core.Part) string {
	t.Helper()

	fp, ok := p.(*core.FilePart)
	require.Truef(t, ok, "expected *core.FilePart, got %T", p)

	switch c := fp.File.(type) {
	case *core.FileRawBytes:
		return string(c.Bytes)
	case *core.FileBase64:
		return c.Base64
	case *core.FilePath:
		return c.Path
	case *core.FileURI:
		return c.URI
	default:
		t.Fatalf("unexpected FilePart content type: %T", c)
		return ""
	}
}

func TestInMemoryArtifactStore_SaveGetIsolation(t *testing.T) {
	svc := NewInMemoryStore()

	// Save a FilePart with raw bytes content (always pointer)
	fp := &core.FilePart{
		File:     &core.FileRawBytes{Bytes: []byte("hello")},
		MimeType: "text/plain",
		Name:     "a1",
	}
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "a1", fp))

	// Mutate original after Save; store should remain unchanged if Save deep-copies
	fp.File = &core.FileRawBytes{Bytes: []byte("HELLO")}

	out, err := svc.Load(context.Background(), "app", "user", "sess", "a1")
	require.NoError(t, err)
	assert.Equal(t, "hello", contentString(t, out)) // should not reflect mutation

	// Mutate returned value; store should remain unchanged if Load returns a copy
	outFP := out.(*core.FilePart)
	outFP.File = &core.FileRawBytes{Bytes: []byte("xxx")}

	out2, err := svc.Load(context.Background(), "app", "user", "sess", "a1")
	require.NoError(t, err)
	assert.Equal(t, "hello", contentString(t, out2)) // original stored should be unchanged
}

func TestInMemoryArtifactStore_ListAndDelete(t *testing.T) {
	svc := NewInMemoryStore()

	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "a1",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("1")}}))
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "a2",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("2")}}))

	ids, err := svc.ListKeys(context.Background(), "app", "user", "sess")
	require.NoError(t, err)
	assert.Len(t, ids, 2)

	require.NoError(t, svc.Delete(context.Background(), "app", "user", "sess", "a1"))

	_, err = svc.Load(context.Background(), "app", "user", "sess", "a1")
	require.Error(t, err)

	ids, _ = svc.ListKeys(context.Background(), "app", "user", "sess")
	assert.Len(t, ids, 1)
}

func TestInMemoryArtifactStore_Concurrency(t *testing.T) {
	svc := NewInMemoryStore()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("a%d", i%10)
			err := svc.Save(context.Background(), "app", "user", "sess", id,
				&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("data")}})
			assert.NoError(t, err)
			_, _ = svc.ListKeys(context.Background(), "app", "user", "sess")
		}()
	}

	wg.Wait()

	ids, err := svc.ListKeys(context.Background(), "app", "user", "sess")
	require.NoError(t, err)
	assert.NotEmpty(t, ids)
}

func TestInMemoryArtifactStore_ListInsertionOrder(t *testing.T) {
	svc := NewInMemoryStore()

	// Insert out of lexicographic order and ensure insertion order is preserved.
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "b",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("1")}}))
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "a",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("2")}}))
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "c",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("3")}}))

	ids, err := svc.ListKeys(context.Background(), "app", "user", "sess")
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "a", "c"}, ids)

	// Overwrite should not duplicate or move the id.
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "a",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("updated")}}))
	ids, err = svc.ListKeys(context.Background(), "app", "user", "sess")
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "a", "c"}, ids)
}

func TestInMemoryArtifactStore_DeleteMaintainsOrder(t *testing.T) {
	svc := NewInMemoryStore()

	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "a",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("1")}}))
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "b",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("2")}}))
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "c",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("3")}}))

	ids, err := svc.ListKeys(context.Background(), "app", "user", "sess")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, ids)

	// Delete middle and verify remaining order.
	require.NoError(t, svc.Delete(context.Background(), "app", "user", "sess", "b"))
	ids, err = svc.ListKeys(context.Background(), "app", "user", "sess")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "c"}, ids)

	// Re-add previously deleted id; it should append at the end.
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "b",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("2b")}}))
	ids, err = svc.ListKeys(context.Background(), "app", "user", "sess")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "c", "b"}, ids)
}

func TestInMemoryArtifactStore_ListSnapshotIndependence(t *testing.T) {
	svc := NewInMemoryStore()

	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "a",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("1")}}))
	require.NoError(t, svc.Save(context.Background(), "app", "user", "sess", "b",
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("2")}}))

	ids, err := svc.ListKeys(context.Background(), "app", "user", "sess")
	require.NoError(t, err)
	require.Len(t, ids, 2)

	// Mutate returned slice; store should be unaffected.
	ids[0] = "zzz"

	ids2, err := svc.ListKeys(context.Background(), "app", "user", "sess")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, ids2)
}
