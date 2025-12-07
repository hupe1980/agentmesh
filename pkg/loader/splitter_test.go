package loader

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecursiveCharacterSplitter_Basic(t *testing.T) {
	t.Parallel()

	splitter := NewRecursiveCharacterSplitter(50, 10)

	doc := Document{
		Content: "This is a test document.\n\nIt has multiple paragraphs.\n\nEach paragraph should be split appropriately.",
		Source:  "test",
	}

	chunks, err := splitter.Split(doc)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)

	// Verify metadata is preserved and enhanced
	for i, chunk := range chunks {
		require.Equal(t, "test", chunk.Source)
		require.Equal(t, i, chunk.Metadata["chunk_index"])
		require.NotEmpty(t, chunk.Content)
	}
}

func TestRecursiveCharacterSplitter_SmallDoc(t *testing.T) {
	t.Parallel()

	splitter := NewRecursiveCharacterSplitter(1000, 100)

	doc := Document{
		Content: "Short document",
		Source:  "test",
	}

	chunks, err := splitter.Split(doc)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.Equal(t, "Short document", chunks[0].Content)
}

func TestRecursiveCharacterSplitter_EmptyDoc(t *testing.T) {
	t.Parallel()

	splitter := NewRecursiveCharacterSplitter(100, 10)

	doc := Document{
		Content: "",
		Source:  "test",
	}

	chunks, err := splitter.Split(doc)
	require.NoError(t, err)
	require.Empty(t, chunks)
}

func TestRecursiveCharacterSplitter_SplitDocuments(t *testing.T) {
	t.Parallel()

	splitter := NewRecursiveCharacterSplitter(50, 0)

	docs := []Document{
		{Content: "First document content", Source: "doc1"},
		{Content: "Second document content", Source: "doc2"},
	}

	chunks, err := splitter.SplitDocuments(docs)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)
}

func TestRecursiveCharacterSplitter_CustomSeparators(t *testing.T) {
	t.Parallel()

	splitter := NewRecursiveCharacterSplitter(100, 0, WithSeparators([]string{".", " ", ""}))

	doc := Document{
		Content: "First sentence. Second sentence. Third sentence.",
		Source:  "test",
	}

	chunks, err := splitter.Split(doc)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)
}

func TestTokenSplitter_Basic(t *testing.T) {
	t.Parallel()

	splitter := NewTokenSplitter(5, 2)

	doc := Document{
		Content: "one two three four five six seven eight nine ten",
		Source:  "test",
	}

	chunks, err := splitter.Split(doc)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)

	// First chunk should have 5 tokens
	tokens := splitter.TokenizerFunc(chunks[0].Content)
	require.Len(t, tokens, 5)
}

func TestTokenSplitter_SmallDoc(t *testing.T) {
	t.Parallel()

	splitter := NewTokenSplitter(100, 10)

	doc := Document{
		Content: "short text",
		Source:  "test",
	}

	chunks, err := splitter.Split(doc)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.Equal(t, "short text", chunks[0].Content)
}

func TestTokenSplitter_SplitDocuments(t *testing.T) {
	t.Parallel()

	splitter := NewTokenSplitter(3, 1)

	docs := []Document{
		{Content: "one two three four five", Source: "doc1"},
		{Content: "a b c d e", Source: "doc2"},
	}

	chunks, err := splitter.SplitDocuments(docs)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)
}
