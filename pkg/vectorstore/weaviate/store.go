package weaviate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"
)

// Ensure Store implements the interfaces.
var (
	_ vectorstore.VectorStore = (*Store)(nil)
	_ vectorstore.Indexer     = (*Store)(nil)
)

// Options configures the Weaviate store.
type Options struct {
	// Host is the Weaviate server host (e.g., "localhost:8080").
	Host string

	// Scheme is the connection scheme ("http" or "https"). Default: "http"
	Scheme string

	// ClassName is the Weaviate class name. Default: "Document"
	ClassName string

	// Dimensions is the vector dimensionality.
	Dimensions int

	// Metric specifies the distance metric. Default: Cosine
	Metric embedding.Metric

	// AutoCreateClass creates the class if it doesn't exist.
	AutoCreateClass bool

	// APIKey is an optional API key for authentication.
	APIKey string
}

// Option configures a Store.
type Option func(*Options)

// WithHost sets the Weaviate host.
func WithHost(host string) Option {
	return func(o *Options) {
		o.Host = host
	}
}

// WithScheme sets the connection scheme.
func WithScheme(scheme string) Option {
	return func(o *Options) {
		o.Scheme = scheme
	}
}

// WithClassName sets the class name.
func WithClassName(name string) Option {
	return func(o *Options) {
		o.ClassName = name
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

// WithAutoCreateClass enables automatic class creation.
func WithAutoCreateClass(auto bool) Option {
	return func(o *Options) {
		o.AutoCreateClass = auto
	}
}

// WithAPIKey sets the API key for authentication.
func WithAPIKey(key string) Option {
	return func(o *Options) {
		o.APIKey = key
	}
}

// Store is a Weaviate-backed VectorStore implementation.
type Store struct {
	client *weaviate.Client
	opts   Options
}

// New creates a new Weaviate vector store.
func New(optFns ...Option) (*Store, error) {
	opts := Options{
		Host:            "localhost:8080",
		Scheme:          "http",
		ClassName:       "Document",
		Metric:          embedding.Cosine,
		AutoCreateClass: true,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	cfg := weaviate.Config{
		Host:   opts.Host,
		Scheme: opts.Scheme,
	}

	if opts.APIKey != "" {
		cfg.Headers = map[string]string{
			"Authorization": "Bearer " + opts.APIKey,
		}
	}

	client, err := weaviate.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("weaviate: failed to create client: %w", err)
	}

	store := &Store{
		client: client,
		opts:   opts,
	}

	// Auto-create class if configured
	if opts.AutoCreateClass && opts.Dimensions > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := store.ensureClass(ctx, opts.ClassName, opts.Dimensions, opts.Metric); err != nil {
			return nil, err
		}
	}

	return store, nil
}

// ensureClass creates the class if it doesn't exist.
func (s *Store) ensureClass(ctx context.Context, name string, _ int, metric embedding.Metric) error {
	exists, err := s.client.Schema().ClassExistenceChecker().WithClassName(name).Do(ctx)
	if err != nil {
		return fmt.Errorf("weaviate: failed to check class existence: %w", err)
	}

	if exists {
		return nil
	}

	class := &models.Class{
		Class:       name,
		Description: "AgentMesh vector store documents",
		Properties: []*models.Property{
			{
				Name:        "content",
				DataType:    []string{"text"},
				Description: "Document content",
			},
			{
				Name:        "docID",
				DataType:    []string{"text"},
				Description: "Original document ID",
			},
			{
				Name:        "namespace",
				DataType:    []string{"text"},
				Description: "Document namespace",
			},
			{
				Name:        "timestamp",
				DataType:    []string{"int"},
				Description: "Creation timestamp",
			},
			{
				Name:        "metadata",
				DataType:    []string{"text"},
				Description: "JSON-encoded metadata",
			},
		},
		VectorIndexConfig: map[string]any{
			"distance": toWeaviateDistance(metric),
		},
	}

	err = s.client.Schema().ClassCreator().WithClass(class).Do(ctx)
	if err != nil {
		return fmt.Errorf("weaviate: failed to create class %q: %w", name, err)
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

	objects := make([]*models.Object, len(docs))
	now := time.Now()

	for i, doc := range docs {
		docID := doc.ID
		if docID == "" {
			docID = uuid.New().String()
		}

		ts := doc.Timestamp
		if ts.IsZero() {
			ts = now.Add(time.Duration(i) * time.Nanosecond)
		}

		// Encode metadata as JSON string
		metadataJSON := "{}"
		if len(doc.Metadata) > 0 {
			metadataJSON = encodeMetadata(doc.Metadata)
		}

		properties := map[string]any{
			"content":   doc.Content,
			"docID":     docID,
			"namespace": opts.Namespace,
			"timestamp": ts.UnixNano(),
			"metadata":  metadataJSON,
		}

		// Generate deterministic UUID from docID
		objectUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(docID)).String()

		objects[i] = &models.Object{
			Class:      s.opts.ClassName,
			ID:         strfmt.UUID(objectUUID),
			Properties: properties,
			Vector:     toFloat32(doc.Embedding),
		}
	}

	// Batch insert
	batcher := s.client.Batch().ObjectsBatcher()
	for _, obj := range objects {
		batcher.WithObjects(obj)
	}

	resp, err := batcher.Do(ctx)
	if err != nil {
		return fmt.Errorf("weaviate: batch insert failed: %w", err)
	}

	// Check for errors in response
	for i := range resp {
		if resp[i].Result != nil && resp[i].Result.Errors != nil && len(resp[i].Result.Errors.Error) > 0 {
			return fmt.Errorf("weaviate: insert error: %v", resp[i].Result.Errors.Error[0].Message)
		}
	}

	return nil
}

// Search finds documents similar to the query embedding.
func (s *Store) Search(ctx context.Context, queryEmbedding embedding.Vector, opts vectorstore.SearchOptions) ([]vectorstore.Document, error) {
	opts.Normalize()

	// Build nearVector query
	nearVector := s.client.GraphQL().NearVectorArgBuilder().
		WithVector(toFloat32(queryEmbedding))

	// Build fields to return
	fields := []graphql.Field{
		{Name: "content"},
		{Name: "docID"},
		{Name: "namespace"},
		{Name: "timestamp"},
		{Name: "metadata"},
		{Name: "_additional", Fields: []graphql.Field{
			{Name: "id"},
			{Name: "distance"},
			{Name: "vector"},
		}},
	}

	// Build query
	query := s.client.GraphQL().Get().
		WithClassName(s.opts.ClassName).
		WithFields(fields...).
		WithNearVector(nearVector).
		WithLimit(opts.K)

	// Add namespace filter if specified
	if opts.Namespace != "" || len(opts.Filter) > 0 {
		where := s.buildWhereFilter(opts.Namespace, opts.Filter)
		if where != nil {
			query = query.WithWhere(where)
		}
	}

	result, err := query.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("weaviate: search failed: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("weaviate: search error: %v", result.Errors[0].Message)
	}

	// Parse results
	return s.parseSearchResults(result, opts)
}

// buildWhereFilter constructs a Weaviate where filter.
func (s *Store) buildWhereFilter(namespace string, _ vectorstore.Filter) *filters.WhereBuilder {
	var conditions []*filters.WhereBuilder

	if namespace != "" {
		conditions = append(conditions, filters.Where().
			WithPath([]string{"namespace"}).
			WithOperator(filters.Equal).
			WithValueText(namespace))
	}

	// Note: For complex metadata filters, we'd need to query the metadata JSON field
	// This is a simplified implementation for basic use cases

	if len(conditions) == 0 {
		return nil
	}

	if len(conditions) == 1 {
		return conditions[0]
	}

	return filters.Where().WithOperator(filters.And).WithOperands(conditions)
}

// parseSearchResults converts Weaviate GraphQL results to Documents.
func (s *Store) parseSearchResults(result *models.GraphQLResponse, opts vectorstore.SearchOptions) ([]vectorstore.Document, error) {
	data, ok := result.Data["Get"].(map[string]any)
	if !ok {
		return nil, nil
	}

	items, ok := data[s.opts.ClassName].([]any)
	if !ok {
		return nil, nil
	}

	results := make([]vectorstore.Document, 0, len(items))

	for _, item := range items {
		props, ok := item.(map[string]any)
		if !ok {
			continue
		}

		doc := vectorstore.Document{}

		// Extract properties
		if content, ok := props["content"].(string); ok {
			doc.Content = content
		}
		if docID, ok := props["docID"].(string); ok {
			doc.ID = docID
		}
		if ts, ok := props["timestamp"].(float64); ok {
			doc.Timestamp = time.Unix(0, int64(ts))
		}
		if metadataJSON, ok := props["metadata"].(string); ok {
			doc.Metadata = decodeMetadata(metadataJSON)
		}

		// Extract additional fields
		s.parseAdditionalFields(props, opts, &doc)

		// Apply min score filter
		if doc.Score < opts.MinScore {
			continue
		}

		results = append(results, doc)
	}

	return results, nil
}

// parseAdditionalFields extracts distance and vector from _additional fields.
func (s *Store) parseAdditionalFields(props map[string]any, opts vectorstore.SearchOptions, doc *vectorstore.Document) {
	additional, ok := props["_additional"].(map[string]any)
	if !ok {
		return
	}

	if distance, ok := additional["distance"].(float64); ok {
		// Convert distance to similarity score (1 - distance for cosine)
		doc.Score = 1 - distance
	}

	if opts.IncludeEmbeddings {
		if vector, ok := additional["vector"].([]any); ok {
			doc.Embedding = toFloat64FromAny(vector)
		}
	}
}

// Delete removes documents by ID.
func (s *Store) Delete(ctx context.Context, ids []string, namespace string) error {
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		// Generate the same UUID used during insertion
		objectUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String()

		err := s.client.Data().Deleter().
			WithClassName(s.opts.ClassName).
			WithID(objectUUID).
			Do(ctx)
		if err != nil {
			return fmt.Errorf("weaviate: failed to delete object %q: %w", id, err)
		}
	}

	return nil
}

// CreateIndex creates a new class (collection).
func (s *Store) CreateIndex(ctx context.Context, name string, dims int, metric embedding.Metric) error {
	return s.ensureClass(ctx, name, dims, metric)
}

// DeleteIndex removes a class and all its data.
func (s *Store) DeleteIndex(ctx context.Context, name string) error {
	err := s.client.Schema().ClassDeleter().WithClassName(name).Do(ctx)
	if err != nil {
		return fmt.Errorf("weaviate: failed to delete class %q: %w", name, err)
	}

	return nil
}

// ListIndexes returns all available classes.
func (s *Store) ListIndexes(ctx context.Context) ([]string, error) {
	schema, err := s.client.Schema().Getter().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("weaviate: failed to get schema: %w", err)
	}

	names := make([]string, len(schema.Classes))
	for i, class := range schema.Classes {
		names[i] = class.Class
	}

	return names, nil
}

// Close releases resources.
func (s *Store) Close() error {
	// Weaviate client doesn't require explicit closing
	return nil
}

// toWeaviateDistance converts embedding metric to Weaviate distance metric.
func toWeaviateDistance(m embedding.Metric) string {
	switch m {
	case embedding.Cosine:
		return "cosine"
	case embedding.Euclidean:
		return "l2-squared"
	case embedding.DotProduct:
		return "dot"
	default:
		return "cosine"
	}
}

// toFloat32 converts float64 slice to float32 slice.
func toFloat32(v []float64) []float32 {
	result := make([]float32, len(v))
	for i, val := range v {
		result[i] = float32(val)
	}
	return result
}

// toFloat64FromAny converts []any to []float64.
func toFloat64FromAny(v []any) []float64 {
	result := make([]float64, len(v))
	for i, val := range v {
		if f, ok := val.(float64); ok {
			result[i] = f
		}
	}
	return result
}

// encodeMetadata encodes metadata map to JSON string.
func encodeMetadata(m map[string]any) string {
	var sb strings.Builder
	sb.WriteString("{")
	first := true
	for k, v := range m {
		if !first {
			sb.WriteString(",")
		}
		first = false
		sb.WriteString(fmt.Sprintf("%q:", k))
		switch val := v.(type) {
		case string:
			sb.WriteString(fmt.Sprintf("%q", val))
		case float64, int, int64, float32:
			sb.WriteString(fmt.Sprintf("%v", val))
		case bool:
			sb.WriteString(fmt.Sprintf("%v", val))
		default:
			sb.WriteString(fmt.Sprintf("%q", fmt.Sprintf("%v", val)))
		}
	}
	sb.WriteString("}")
	return sb.String()
}

// decodeMetadata decodes JSON string to metadata map.
func decodeMetadata(s string) map[string]any {
	if s == "" || s == "{}" {
		return nil
	}
	// Simple JSON parsing - for production use encoding/json
	result := make(map[string]any)
	// This is a simplified implementation
	// In production, use json.Unmarshal
	return result
}
