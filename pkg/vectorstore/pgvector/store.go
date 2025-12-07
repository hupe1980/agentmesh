package pgvector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// Ensure Store implements the interfaces.
var (
	_ vectorstore.VectorStore = (*Store)(nil)
	_ vectorstore.Indexer     = (*Store)(nil)
)

// Options configures the pgvector store.
type Options struct {
	// TableName is the default table to use. Default: "documents"
	TableName string

	// Dimensions is the vector dimensionality. Required for table creation.
	Dimensions int

	// Metric specifies the distance metric. Default: Cosine
	Metric embedding.Metric

	// AutoCreateTable creates the table if it doesn't exist.
	AutoCreateTable bool
}

// Option configures a Store.
type Option func(*Options)

// WithTableName sets the default table name.
func WithTableName(name string) Option {
	return func(o *Options) {
		o.TableName = name
	}
}

// WithDimensions sets the vector dimensions.
func WithDimensions(dims int) Option {
	return func(o *Options) {
		o.Dimensions = dims
	}
}

// WithMetric sets the distance metric.
func WithMetric(metric embedding.Metric) Option {
	return func(o *Options) {
		o.Metric = metric
	}
}

// WithAutoCreateTable enables automatic table creation.
func WithAutoCreateTable(auto bool) Option {
	return func(o *Options) {
		o.AutoCreateTable = auto
	}
}

// Store is a pgvector-backed VectorStore implementation.
type Store struct {
	pool *pgxpool.Pool
	opts Options
}

// New creates a new pgvector vector store.
// connString should be a PostgreSQL connection string.
func New(ctx context.Context, connString string, optFns ...Option) (*Store, error) {
	opts := Options{
		TableName:       "documents",
		Metric:          embedding.Cosine,
		AutoCreateTable: true,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Enable pgvector extension
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to enable vector extension: %w", err)
	}

	store := &Store{
		pool: pool,
		opts: opts,
	}

	// Auto-create table if configured
	if opts.AutoCreateTable && opts.Dimensions > 0 {
		if err := store.ensureTable(ctx, opts.TableName, opts.Dimensions, opts.Metric); err != nil {
			pool.Close()
			return nil, err
		}
	}

	return store, nil
}

// ensureTable creates a table if it doesn't exist.
func (s *Store) ensureTable(ctx context.Context, name string, dims int, metric embedding.Metric) error {
	// Create table
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content TEXT,
			embedding vector(%d),
			metadata JSONB,
			namespace TEXT DEFAULT '',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`, sanitizeIdentifier(name), dims)

	_, err := s.pool.Exec(ctx, createSQL)
	if err != nil {
		return fmt.Errorf("failed to create table %q: %w", name, err)
	}

	// Create index for vector similarity search
	indexName := fmt.Sprintf("%s_embedding_idx", name)
	opClass := metricToOpClass(metric)

	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS %s ON %s 
		USING ivfflat (embedding %s)
		WITH (lists = 100)
	`, sanitizeIdentifier(indexName), sanitizeIdentifier(name), opClass)

	_, err = s.pool.Exec(ctx, indexSQL)
	if err != nil {
		// IVFFlat index requires data, try HNSW instead
		indexSQL = fmt.Sprintf(`
			CREATE INDEX IF NOT EXISTS %s ON %s 
			USING hnsw (embedding %s)
		`, sanitizeIdentifier(indexName), sanitizeIdentifier(name), opClass)
		_, err = s.pool.Exec(ctx, indexSQL)
		if err != nil {
			// Index creation failed, but table exists - continue without index
			// This is acceptable for small datasets
			return nil //nolint:nilerr // Table exists, index is optional
		}
	}

	return nil
}

// Add inserts or updates documents in the store.
func (s *Store) Add(ctx context.Context, docs []vectorstore.Document, optFns ...func(*vectorstore.AddOptions)) error {
	if len(docs) == 0 {
		return nil
	}

	opts := vectorstore.AddOptions{
		Upsert: true,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	tableName := s.tableName(opts.Namespace)

	// Auto-create table if enabled
	if s.opts.AutoCreateTable && s.opts.Dimensions > 0 {
		if err := s.ensureTable(ctx, tableName, s.opts.Dimensions, s.opts.Metric); err != nil {
			return err
		}
	}

	// Use batch insert
	batch := &pgx.Batch{}
	now := time.Now()

	for i, doc := range docs {
		id := doc.ID
		if id == "" {
			id = uuid.New().String()
		}

		ts := doc.Timestamp
		if ts.IsZero() {
			ts = now.Add(time.Duration(i) * time.Nanosecond)
		}

		metadataJSON, err := json.Marshal(doc.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		vec := pgvector.NewVector(toFloat32(doc.Embedding))

		sql := fmt.Sprintf(`
			INSERT INTO %s (id, content, embedding, metadata, namespace, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				embedding = EXCLUDED.embedding,
				metadata = EXCLUDED.metadata,
				created_at = EXCLUDED.created_at
		`, sanitizeIdentifier(tableName))

		batch.Queue(sql, id, doc.Content, vec, metadataJSON, opts.Namespace, ts)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	for range docs {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("failed to insert document: %w", err)
		}
	}

	return nil
}

// Search finds documents similar to the query embedding.
func (s *Store) Search(ctx context.Context, queryEmbedding embedding.Vector, opts vectorstore.SearchOptions) ([]vectorstore.Document, error) {
	opts.Normalize()
	tableName := s.tableName(opts.Namespace)

	vec := pgvector.NewVector(toFloat32(queryEmbedding))
	distanceOp := metricToOperator(s.opts.Metric)

	// Build query - pre-allocate for namespace + filter entries
	conditions := make([]string, 0, 1+len(opts.Filter))
	args := make([]any, 0, 1+1+len(opts.Filter)*2)

	args = append(args, vec)
	argIdx := 2

	if opts.Namespace != "" {
		conditions = append(conditions, fmt.Sprintf("namespace = $%d", argIdx))
		args = append(args, opts.Namespace)
		argIdx++
	}

	// Add metadata filters
	for k, v := range opts.Filter {
		conditions = append(conditions, fmt.Sprintf("metadata->>$%d = $%d", argIdx, argIdx+1))
		args = append(args, k, fmt.Sprintf("%v", v))
		argIdx += 2
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Build select columns
	selectCols := "id, content, metadata, namespace, created_at"
	if opts.IncludeEmbeddings {
		selectCols = "id, content, embedding, metadata, namespace, created_at"
	}

	query := fmt.Sprintf(`
		SELECT %s, (embedding %s $1) as distance
		FROM %s
		%s
		ORDER BY embedding %s $1
		LIMIT %d
	`, selectCols, distanceOp, sanitizeIdentifier(tableName), whereClause, distanceOp, opts.K)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	results := make([]vectorstore.Document, 0)

	for rows.Next() {
		doc, err := s.scanDocument(rows, opts.IncludeEmbeddings)
		if err != nil {
			return nil, err
		}

		// Filter by min score (convert distance to score)
		score := distanceToScore(doc.Score, s.opts.Metric)
		if score < opts.MinScore {
			continue
		}

		doc.Score = score
		results = append(results, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating results: %w", err)
	}

	return results, nil
}

// scanDocument scans a row into a Document.
func (s *Store) scanDocument(rows pgx.Rows, includeEmbedding bool) (vectorstore.Document, error) {
	var doc vectorstore.Document
	var metadataJSON []byte
	var distance float64
	var namespace string
	var createdAt time.Time

	if includeEmbedding {
		var vec pgvector.Vector
		err := rows.Scan(&doc.ID, &doc.Content, &vec, &metadataJSON, &namespace, &createdAt, &distance)
		if err != nil {
			return doc, fmt.Errorf("failed to scan row: %w", err)
		}
		doc.Embedding = toFloat64(vec.Slice())
	} else {
		err := rows.Scan(&doc.ID, &doc.Content, &metadataJSON, &namespace, &createdAt, &distance)
		if err != nil {
			return doc, fmt.Errorf("failed to scan row: %w", err)
		}
	}

	doc.Timestamp = createdAt
	doc.Score = distance // Will be converted to score later

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &doc.Metadata); err != nil {
			return doc, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return doc, nil
}

// Delete removes documents by ID.
func (s *Store) Delete(ctx context.Context, ids []string, namespace string) error {
	if len(ids) == 0 {
		return nil
	}

	tableName := s.tableName(namespace)

	// Build placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)",
		sanitizeIdentifier(tableName),
		strings.Join(placeholders, ", "))

	_, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete documents: %w", err)
	}

	return nil
}

// Close releases resources.
func (s *Store) Close() error {
	s.pool.Close()
	return nil
}

// CreateIndex creates a new table with vector column.
func (s *Store) CreateIndex(ctx context.Context, name string, dims int, metric embedding.Metric) error {
	return s.ensureTable(ctx, name, dims, metric)
}

// DeleteIndex removes a table and all its data.
func (s *Store) DeleteIndex(ctx context.Context, name string) error {
	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", sanitizeIdentifier(name))
	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop table %q: %w", name, err)
	}
	return nil
}

// ListIndexes returns all tables with vector columns.
func (s *Store) ListIndexes(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT table_name 
		FROM information_schema.columns 
		WHERE udt_name = 'vector' 
		AND table_schema = 'public'
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, name)
	}

	return tables, nil
}

// tableName returns the table name, using namespace as suffix if provided.
func (s *Store) tableName(namespace string) string {
	if namespace == "" {
		return s.opts.TableName
	}
	return s.opts.TableName + "_" + namespace
}

// Helper functions

func sanitizeIdentifier(name string) string {
	// Basic SQL injection prevention - only allow alphanumeric and underscore
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func metricToOpClass(metric embedding.Metric) string {
	switch metric {
	case embedding.Euclidean:
		return "vector_l2_ops"
	case embedding.DotProduct:
		return "vector_ip_ops"
	default: // Cosine
		return "vector_cosine_ops"
	}
}

func metricToOperator(metric embedding.Metric) string {
	switch metric {
	case embedding.Euclidean:
		return "<->"
	case embedding.DotProduct:
		return "<#>"
	default: // Cosine
		return "<=>"
	}
}

func distanceToScore(distance float64, metric embedding.Metric) float64 {
	switch metric {
	case embedding.Euclidean:
		// L2 distance: smaller is better, convert to 0-1 range
		return 1.0 / (1.0 + distance)
	case embedding.DotProduct:
		// Inner product: pgvector returns negative, negate to get similarity
		return -distance
	default: // Cosine
		// Cosine distance: 0 means identical, 2 means opposite
		return 1.0 - distance
	}
}

func toFloat32(vec []float64) []float32 {
	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(v)
	}
	return result
}

func toFloat64(vec []float32) []float64 {
	result := make([]float64, len(vec))
	for i, v := range vec {
		result[i] = float64(v)
	}
	return result
}
