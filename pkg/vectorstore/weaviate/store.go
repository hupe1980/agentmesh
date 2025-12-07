package weaviate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/internal/floatconv"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	weaviateclient "github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"
)

// Ensure Store implements the interfaces.
var (
	_ vectorstore.VectorStore = (*Store)(nil)
	_ vectorstore.Indexer     = (*Store)(nil)
)

// Client defines the interface for Weaviate operations.
// This interface allows for mocking in tests.
type Client interface {
	// Schema operations
	ClassExists(ctx context.Context, className string) (bool, error)
	CreateClass(ctx context.Context, class *models.Class) error
	DeleteClass(ctx context.Context, className string) error
	GetSchema(ctx context.Context) (*models.Schema, error)

	// Batch operations
	BatchObjects(ctx context.Context, objects []*models.Object) ([]models.ObjectsGetResponse, error)

	// Data operations
	DeleteObject(ctx context.Context, className, id string) error

	// GraphQL operations
	GraphQLQuery(ctx context.Context, className string, fields []graphql.Field, nearVector []float32, limit int, where *filters.WhereBuilder) (*models.GraphQLResponse, error)
}

// Options configures the Weaviate store.
type Options struct {
	// ClassName is the Weaviate class name. Default: "Document"
	ClassName string

	// Metric specifies the distance metric. Default: Cosine
	Metric embedding.Metric
}

// Option configures a Store.
type Option func(*Options)

// WithClassName sets the class name.
func WithClassName(name string) Option {
	return func(o *Options) {
		o.ClassName = name
	}
}

// WithMetric sets the distance metric.
func WithMetric(metric embedding.Metric) Option {
	return func(o *Options) {
		o.Metric = metric
	}
}

// Store is a Weaviate-backed VectorStore implementation.
type Store struct {
	client Client
	opts   Options
}

// NewFromClient creates a new Weaviate vector store with the provided client.
func NewFromClient(client Client, optFns ...Option) *Store {
	opts := Options{
		ClassName: "Document",
		Metric:    embedding.Cosine,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	return &Store{
		client: client,
		opts:   opts,
	}
}

// New creates a new Weaviate vector store from configuration.
// host is the Weaviate host (e.g., "localhost:8080").
// scheme is the HTTP scheme ("http" or "https").
func New(ctx context.Context, host, scheme string, optFns ...Option) (*Store, error) {
	cfg := weaviateclient.Config{
		Host:   host,
		Scheme: scheme,
	}

	weaviateClient, err := weaviateclient.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("weaviate: failed to create client: %w", err)
	}

	client := &weaviateClientWrapper{client: weaviateClient}
	store := NewFromClient(client, optFns...)

	return store, nil
}

// weaviateClientWrapper wraps the weaviate client to implement the Client interface.
type weaviateClientWrapper struct {
	client *weaviateclient.Client
}

// ClassExists checks if a class exists.
func (w *weaviateClientWrapper) ClassExists(ctx context.Context, className string) (bool, error) {
	return w.client.Schema().ClassExistenceChecker().WithClassName(className).Do(ctx)
}

// CreateClass creates a new class.
func (w *weaviateClientWrapper) CreateClass(ctx context.Context, class *models.Class) error {
	return w.client.Schema().ClassCreator().WithClass(class).Do(ctx)
}

// DeleteClass deletes a class.
func (w *weaviateClientWrapper) DeleteClass(ctx context.Context, className string) error {
	return w.client.Schema().ClassDeleter().WithClassName(className).Do(ctx)
}

// GetSchema returns the schema.
func (w *weaviateClientWrapper) GetSchema(ctx context.Context) (*models.Schema, error) {
	dump, err := w.client.Schema().Getter().Do(ctx)
	if err != nil {
		return nil, err
	}
	return &models.Schema{Classes: dump.Classes}, nil
}

// BatchObjects inserts objects in batch.
func (w *weaviateClientWrapper) BatchObjects(ctx context.Context, objects []*models.Object) ([]models.ObjectsGetResponse, error) {
	return w.client.Batch().ObjectsBatcher().WithObjects(objects...).Do(ctx)
}

// DeleteObject deletes an object.
func (w *weaviateClientWrapper) DeleteObject(ctx context.Context, className, id string) error {
	return w.client.Data().Deleter().
		WithClassName(className).
		WithID(id).
		Do(ctx)
}

// GraphQLQuery performs a GraphQL query.
func (w *weaviateClientWrapper) GraphQLQuery(ctx context.Context, className string, fields []graphql.Field, nearVector []float32, limit int, where *filters.WhereBuilder) (*models.GraphQLResponse, error) {
	builder := w.client.GraphQL().Get().
		WithClassName(className).
		WithFields(fields...).
		WithNearVector(w.client.GraphQL().NearVectorArgBuilder().WithVector(nearVector)).
		WithLimit(limit)

	if where != nil {
		builder = builder.WithWhere(where)
	}

	return builder.Do(ctx)
}

// EnsureClass creates the class if it doesn't exist.
func (s *Store) EnsureClass(ctx context.Context, name string, metric embedding.Metric) error {
	exists, err := s.client.ClassExists(ctx, name)
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

	err = s.client.CreateClass(ctx, class)
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
			Vector:     floatconv.ToFloat32(doc.Embedding),
		}
	}

	resp, err := s.client.BatchObjects(ctx, objects)
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

	// Build where filter if needed
	var where *filters.WhereBuilder
	if opts.Namespace != "" || len(opts.Filter) > 0 {
		where = s.buildWhereFilter(opts.Namespace, opts.Filter)
	}

	result, err := s.client.GraphQLQuery(ctx, s.opts.ClassName, fields, floatconv.ToFloat32(queryEmbedding), opts.K, where)
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
			doc.Embedding = floatconv.ToFloat64FromAny(vector)
		}
	}
}

// Delete removes documents by ID.
func (s *Store) Delete(ctx context.Context, ids []string, _ string) error {
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		// Generate the same UUID used during insertion
		objectUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String()

		err := s.client.DeleteObject(ctx, s.opts.ClassName, objectUUID)
		if err != nil {
			return fmt.Errorf("weaviate: failed to delete object %q: %w", id, err)
		}
	}

	return nil
}

// CreateIndex creates a new class (collection).
func (s *Store) CreateIndex(ctx context.Context, name string, _ int, metric embedding.Metric) error {
	return s.EnsureClass(ctx, name, metric)
}

// DeleteIndex removes a class and all its data.
func (s *Store) DeleteIndex(ctx context.Context, name string) error {
	err := s.client.DeleteClass(ctx, name)
	if err != nil {
		return fmt.Errorf("weaviate: failed to delete class %q: %w", name, err)
	}

	return nil
}

// ListIndexes returns all available classes.
func (s *Store) ListIndexes(ctx context.Context) ([]string, error) {
	schema, err := s.client.GetSchema(ctx)
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
	result := make(map[string]any)
	return result
}
