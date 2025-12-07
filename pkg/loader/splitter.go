package loader

import (
	"maps"
	"strings"
	"unicode/utf8"
)

// Splitter splits documents into smaller chunks.
type Splitter interface {
	// Split divides a document into smaller chunks.
	Split(doc Document) ([]Document, error)

	// SplitDocuments splits multiple documents.
	SplitDocuments(docs []Document) ([]Document, error)
}

// RecursiveCharacterSplitter splits text by recursively trying different separators.
// It attempts to keep semantically related text together by trying separators
// in order of decreasing granularity (paragraphs → lines → words → characters).
type RecursiveCharacterSplitter struct {
	// ChunkSize is the maximum size of each chunk in characters.
	ChunkSize int

	// ChunkOverlap is the number of characters to overlap between chunks.
	ChunkOverlap int

	// Separators are tried in order until the text is small enough.
	Separators []string

	// LengthFunc calculates the length of a string (default: utf8.RuneCountInString).
	LengthFunc func(string) int

	// KeepSeparator determines whether to keep the separator in the chunks.
	KeepSeparator bool
}

// NewRecursiveCharacterSplitter creates a splitter with sensible defaults.
func NewRecursiveCharacterSplitter(chunkSize, chunkOverlap int, opts ...SplitterOption) *RecursiveCharacterSplitter {
	s := &RecursiveCharacterSplitter{
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
		Separators:   []string{"\n\n", "\n", " ", ""},
		LengthFunc:   utf8.RuneCountInString,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SplitterOption configures a RecursiveCharacterSplitter.
type SplitterOption func(*RecursiveCharacterSplitter)

// WithSeparators sets custom separators.
func WithSeparators(separators []string) SplitterOption {
	return func(s *RecursiveCharacterSplitter) {
		s.Separators = separators
	}
}

// WithLengthFunc sets a custom length function.
func WithLengthFunc(f func(string) int) SplitterOption {
	return func(s *RecursiveCharacterSplitter) {
		s.LengthFunc = f
	}
}

// WithKeepSeparator configures whether to keep separators in output.
func WithKeepSeparator(keep bool) SplitterOption {
	return func(s *RecursiveCharacterSplitter) {
		s.KeepSeparator = keep
	}
}

// Split splits a single document into chunks.
func (s *RecursiveCharacterSplitter) Split(doc Document) ([]Document, error) {
	chunks := s.splitText(doc.Content, s.Separators)

	docs := make([]Document, 0, len(chunks))
	for i, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			continue
		}

		meta := make(map[string]any, len(doc.Metadata)+2)
		maps.Copy(meta, doc.Metadata)
		meta["chunk_index"] = i
		meta["chunk_count"] = len(chunks)

		docs = append(docs, Document{
			Content:  chunk,
			Metadata: meta,
			Source:   doc.Source,
		})
	}

	return docs, nil
}

// SplitDocuments splits multiple documents.
func (s *RecursiveCharacterSplitter) SplitDocuments(docs []Document) ([]Document, error) {
	var result []Document
	for _, doc := range docs {
		chunks, err := s.Split(doc)
		if err != nil {
			return nil, err
		}
		result = append(result, chunks...)
	}
	return result, nil
}

// splitText recursively splits text using the given separators.
func (s *RecursiveCharacterSplitter) splitText(text string, separators []string) []string {
	if len(separators) == 0 {
		return []string{text}
	}

	separator := separators[0]
	newSeparators := separators[1:]

	splits := s.performSplit(text, separator)
	goodSplits := s.processSplits(splits, separator, newSeparators)

	return s.mergeWithOverlap(goodSplits)
}

// performSplit splits text by the given separator.
func (s *RecursiveCharacterSplitter) performSplit(text, separator string) []string {
	if separator == "" {
		return s.splitByRune(text)
	}
	return strings.Split(text, separator)
}

// processSplits processes splits and accumulates chunks.
func (s *RecursiveCharacterSplitter) processSplits(splits []string, separator string, newSeparators []string) []string {
	var goodSplits []string
	var currentDoc strings.Builder

	for i, split := range splits {
		piece := s.preparePiece(split, separator, i)

		if s.LengthFunc(piece) < s.ChunkSize {
			goodSplits, currentDoc = s.handleSmallPiece(piece, separator, goodSplits, currentDoc)
		} else {
			goodSplits, currentDoc = s.handleLargePiece(piece, newSeparators, goodSplits, currentDoc)
		}
	}

	if currentDoc.Len() > 0 {
		goodSplits = append(goodSplits, currentDoc.String())
	}

	return goodSplits
}

// preparePiece prepares a piece with separator if needed.
func (s *RecursiveCharacterSplitter) preparePiece(split, separator string, index int) string {
	if s.KeepSeparator && separator != "" && index > 0 {
		return separator + split
	}
	return split
}

// handleSmallPiece handles a piece that fits within chunk size.
func (s *RecursiveCharacterSplitter) handleSmallPiece(piece, separator string, goodSplits []string, currentDoc strings.Builder) ([]string, strings.Builder) {
	if currentDoc.Len() > 0 {
		combined := s.buildCombined(currentDoc.String(), piece, separator)
		if s.LengthFunc(combined) > s.ChunkSize {
			goodSplits = append(goodSplits, currentDoc.String())
			currentDoc.Reset()
		}
	}

	if currentDoc.Len() > 0 && separator != "" && !s.KeepSeparator {
		currentDoc.WriteString(separator)
	}
	currentDoc.WriteString(piece)

	return goodSplits, currentDoc
}

// buildCombined builds the combined string for size checking.
func (s *RecursiveCharacterSplitter) buildCombined(current, piece, separator string) string {
	if !s.KeepSeparator && separator != "" {
		return current + separator + piece
	}
	return current + piece
}

// handleLargePiece handles a piece that exceeds chunk size.
func (s *RecursiveCharacterSplitter) handleLargePiece(piece string, newSeparators []string, goodSplits []string, currentDoc strings.Builder) ([]string, strings.Builder) {
	if currentDoc.Len() > 0 {
		goodSplits = append(goodSplits, currentDoc.String())
		currentDoc.Reset()
	}

	if len(newSeparators) > 0 {
		subSplits := s.splitText(piece, newSeparators)
		goodSplits = append(goodSplits, subSplits...)
	} else {
		goodSplits = append(goodSplits, piece)
	}

	return goodSplits, currentDoc
}

// splitByRune splits text into individual runes.
func (s *RecursiveCharacterSplitter) splitByRune(text string) []string {
	runes := []rune(text)
	result := make([]string, len(runes))
	for i, r := range runes {
		result[i] = string(r)
	}
	return result
}

// mergeWithOverlap merges chunks with the specified overlap.
func (s *RecursiveCharacterSplitter) mergeWithOverlap(chunks []string) []string {
	if len(chunks) <= 1 || s.ChunkOverlap <= 0 {
		return chunks
	}

	result := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		if i == 0 {
			result = append(result, chunk)
			continue
		}

		// Get overlap from previous chunk
		prev := chunks[i-1]
		prevRunes := []rune(prev)
		overlapStart := max(len(prevRunes)-s.ChunkOverlap, 0)
		overlap := string(prevRunes[overlapStart:])

		// Prepend overlap to current chunk
		result = append(result, overlap+chunk)
	}

	return result
}

// TokenSplitter splits text by token count.
// Useful for ensuring chunks fit within model context windows.
type TokenSplitter struct {
	// ChunkSize is the maximum number of tokens per chunk.
	ChunkSize int

	// ChunkOverlap is the number of tokens to overlap.
	ChunkOverlap int

	// TokenizerFunc converts text to tokens.
	TokenizerFunc func(string) []string

	// DetokenizerFunc converts tokens back to text.
	DetokenizerFunc func([]string) string
}

// NewTokenSplitter creates a token-based splitter.
// Uses a simple whitespace tokenizer by default.
func NewTokenSplitter(chunkSize, chunkOverlap int) *TokenSplitter {
	return &TokenSplitter{
		ChunkSize:     chunkSize,
		ChunkOverlap:  chunkOverlap,
		TokenizerFunc: strings.Fields,
		DetokenizerFunc: func(tokens []string) string {
			return strings.Join(tokens, " ")
		},
	}
}

// Split splits a document by token count.
func (s *TokenSplitter) Split(doc Document) ([]Document, error) {
	tokens := s.TokenizerFunc(doc.Content)

	var docs []Document
	chunkIndex := 0

	for i := 0; i < len(tokens); i += s.ChunkSize - s.ChunkOverlap {
		end := min(i+s.ChunkSize, len(tokens))

		chunk := s.DetokenizerFunc(tokens[i:end])
		if strings.TrimSpace(chunk) == "" {
			continue
		}

		meta := make(map[string]any, len(doc.Metadata)+3)
		maps.Copy(meta, doc.Metadata)
		meta["chunk_index"] = chunkIndex
		meta["start_token"] = i
		meta["end_token"] = end

		docs = append(docs, Document{
			Content:  chunk,
			Metadata: meta,
			Source:   doc.Source,
		})
		chunkIndex++

		if end >= len(tokens) {
			break
		}
	}

	return docs, nil
}

// SplitDocuments splits multiple documents.
func (s *TokenSplitter) SplitDocuments(docs []Document) ([]Document, error) {
	var result []Document
	for _, doc := range docs {
		chunks, err := s.Split(doc)
		if err != nil {
			return nil, err
		}
		result = append(result, chunks...)
	}
	return result, nil
}
