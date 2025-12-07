package loader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReaderLoader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	content := "Hello, World!"
	reader := strings.NewReader(content)

	loader := NewReaderLoader(reader,
		WithReaderSource("test"),
		WithReaderMetadata(map[string]any{"key": "value"}),
	)

	docs, err := loader.Load(ctx)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, content, docs[0].Content)
	require.Equal(t, "test", docs[0].Source)
	require.Equal(t, "value", docs[0].Metadata["key"])
}

func TestFileLoader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := "File content here"

	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))

	loader := NewFileLoader(filePath)
	docs, err := loader.Load(ctx)

	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, content, docs[0].Content)
	require.Equal(t, filePath, docs[0].Source)
	require.Equal(t, "test.txt", docs[0].Metadata["filename"])
	require.Equal(t, ".txt", docs[0].Metadata["extension"])
}

func TestDirectoryLoader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create temp directory with files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file3.md"), []byte("markdown"), 0o600))

	// Load all files
	loader := NewDirectoryLoader(tmpDir)
	docs, err := loader.Load(ctx)
	require.NoError(t, err)
	require.Len(t, docs, 3)

	// Load only .txt files
	loader = NewDirectoryLoader(tmpDir, WithPattern("*.txt"))
	docs, err = loader.Load(ctx)
	require.NoError(t, err)
	require.Len(t, docs, 2)
}

func TestDirectoryLoaderRecursive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create nested directory structure
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.txt"), []byte("root"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested"), 0o600))

	// Non-recursive should only get root file
	loader := NewDirectoryLoader(tmpDir, WithPattern("*.txt"))
	docs, err := loader.Load(ctx)
	require.NoError(t, err)
	require.Len(t, docs, 1)

	// Recursive should get both
	loader = NewDirectoryLoader(tmpDir, WithPattern("*.txt"), WithRecursive(true))
	docs, err = loader.Load(ctx)
	require.NoError(t, err)
	require.Len(t, docs, 2)
}

func TestStringLoader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	content := "Direct string content"

	loader := NewStringLoader(content, WithStringSource("inline"))
	docs, err := loader.Load(ctx)

	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, content, docs[0].Content)
	require.Equal(t, "inline", docs[0].Source)
}

func TestFunc(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	loader := Func(func(_ context.Context) ([]Document, error) {
		return []Document{{Content: "from func"}}, nil
	})

	docs, err := loader.Load(ctx)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "from func", docs[0].Content)
}

func TestDocumentMethods(t *testing.T) {
	t.Parallel()

	doc := NewDocument("content")
	require.Equal(t, "content", doc.Content)
	require.NotNil(t, doc.Metadata)

	doc = doc.WithMetadata("key", "value")
	require.Equal(t, "value", doc.Metadata["key"])

	doc = doc.WithSource("source")
	require.Equal(t, "source", doc.Source)
}
