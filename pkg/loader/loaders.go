package loader

import (
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// ReaderLoader loads a document from an io.Reader.
type ReaderLoader struct {
	reader   io.Reader
	metadata map[string]any
	source   string
}

// NewReaderLoader creates a loader that reads from an io.Reader.
func NewReaderLoader(reader io.Reader, opts ...ReaderLoaderOption) *ReaderLoader {
	l := &ReaderLoader{
		reader:   reader,
		metadata: make(map[string]any),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// ReaderLoaderOption configures a ReaderLoader.
type ReaderLoaderOption func(*ReaderLoader)

// WithReaderMetadata sets metadata for the loaded document.
func WithReaderMetadata(metadata map[string]any) ReaderLoaderOption {
	return func(l *ReaderLoader) {
		l.metadata = metadata
	}
}

// WithReaderSource sets the source identifier for the loaded document.
func WithReaderSource(source string) ReaderLoaderOption {
	return func(l *ReaderLoader) {
		l.source = source
	}
}

// Load reads the entire content from the reader.
func (l *ReaderLoader) Load(_ context.Context) ([]Document, error) {
	data, err := io.ReadAll(l.reader)
	if err != nil {
		return nil, err
	}

	return []Document{{
		Content:  string(data),
		Metadata: l.metadata,
		Source:   l.source,
	}}, nil
}

// FileLoader loads a document from a local file.
type FileLoader struct {
	path     string
	metadata map[string]any
}

// NewFileLoader creates a loader that reads from a file path.
func NewFileLoader(path string, opts ...FileLoaderOption) *FileLoader {
	l := &FileLoader{
		path:     path,
		metadata: make(map[string]any),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// FileLoaderOption configures a FileLoader.
type FileLoaderOption func(*FileLoader)

// WithFileMetadata sets metadata for the loaded document.
func WithFileMetadata(metadata map[string]any) FileLoaderOption {
	return func(l *FileLoader) {
		l.metadata = metadata
	}
}

// Load reads the file content.
func (l *FileLoader) Load(_ context.Context) ([]Document, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, err
	}

	meta := make(map[string]any, len(l.metadata)+2)
	maps.Copy(meta, l.metadata)
	meta["filename"] = filepath.Base(l.path)
	meta["extension"] = filepath.Ext(l.path)

	return []Document{{
		Content:  string(data),
		Metadata: meta,
		Source:   l.path,
	}}, nil
}

// DirectoryLoader loads all files from a directory with optional filtering.
type DirectoryLoader struct {
	path      string
	pattern   string // glob pattern for filtering (e.g., "*.md")
	recursive bool
	metadata  map[string]any
}

// NewDirectoryLoader creates a loader that reads all matching files from a directory.
func NewDirectoryLoader(path string, opts ...DirectoryLoaderOption) *DirectoryLoader {
	l := &DirectoryLoader{
		path:      path,
		pattern:   "*",
		recursive: false,
		metadata:  make(map[string]any),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// DirectoryLoaderOption configures a DirectoryLoader.
type DirectoryLoaderOption func(*DirectoryLoader)

// WithPattern sets the glob pattern for filtering files.
func WithPattern(pattern string) DirectoryLoaderOption {
	return func(l *DirectoryLoader) {
		l.pattern = pattern
	}
}

// WithRecursive enables recursive directory traversal.
func WithRecursive(recursive bool) DirectoryLoaderOption {
	return func(l *DirectoryLoader) {
		l.recursive = recursive
	}
}

// WithDirectoryMetadata sets metadata for all loaded documents.
func WithDirectoryMetadata(metadata map[string]any) DirectoryLoaderOption {
	return func(l *DirectoryLoader) {
		l.metadata = metadata
	}
}

// Load reads all matching files from the directory.
func (l *DirectoryLoader) Load(ctx context.Context) ([]Document, error) {
	var docs []Document

	walkFn := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip directories
		if d.IsDir() {
			if !l.recursive && path != l.path {
				return filepath.SkipDir
			}
			return nil
		}

		// Check pattern match
		matched, err := filepath.Match(l.pattern, d.Name())
		if err != nil {
			return err
		}
		if !matched {
			return nil
		}

		// Read file
		data, err := os.ReadFile(path) //nolint:gosec // G304: path is validated via WalkDir
		if err != nil {
			return err
		}

		// Build metadata
		meta := make(map[string]any, len(l.metadata)+3)
		maps.Copy(meta, l.metadata)
		meta["filename"] = d.Name()
		meta["extension"] = filepath.Ext(d.Name())
		meta["relative_path"] = strings.TrimPrefix(path, l.path+string(filepath.Separator))

		docs = append(docs, Document{
			Content:  string(data),
			Metadata: meta,
			Source:   path,
		})

		return nil
	}

	if err := filepath.WalkDir(l.path, walkFn); err != nil {
		return nil, err
	}

	return docs, nil
}

// StringLoader creates a document from a string.
type StringLoader struct {
	content  string
	metadata map[string]any
	source   string
}

// NewStringLoader creates a loader from a string.
func NewStringLoader(content string, opts ...StringLoaderOption) *StringLoader {
	l := &StringLoader{
		content:  content,
		metadata: make(map[string]any),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// StringLoaderOption configures a StringLoader.
type StringLoaderOption func(*StringLoader)

// WithStringMetadata sets metadata for the loaded document.
func WithStringMetadata(metadata map[string]any) StringLoaderOption {
	return func(l *StringLoader) {
		l.metadata = metadata
	}
}

// WithStringSource sets the source identifier.
func WithStringSource(source string) StringLoaderOption {
	return func(l *StringLoader) {
		l.source = source
	}
}

// Load returns the string as a document.
func (l *StringLoader) Load(_ context.Context) ([]Document, error) {
	return []Document{{
		Content:  l.content,
		Metadata: l.metadata,
		Source:   l.source,
	}}, nil
}
