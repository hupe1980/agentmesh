package qdrant

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/internal/floatconv"
	"github.com/hupe1980/agentmesh/internal/safeconv"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Ensure Store implements the interfaces.
var (
	_ vectorstore.VectorStore  = (*Store)(nil)
	_ vectorstore.Indexer      = (*Store)(nil)
	_ vectorstore.TextSearcher = (*Store)(nil)
)

// PointsClient defines the interface for Qdrant points operations.
type PointsClient interface {
	Upsert(ctx context.Context, in *qdrant.UpsertPoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error)
	Search(ctx context.Context, in *qdrant.SearchPoints, opts ...grpc.CallOption) (*qdrant.SearchResponse, error)
	Query(ctx context.Context, in *qdrant.QueryPoints, opts ...grpc.CallOption) (*qdrant.QueryResponse, error)
	Delete(ctx context.Context, in *qdrant.DeletePoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error)
	CreateFieldIndex(ctx context.Context, in *qdrant.CreateFieldIndexCollection, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error)
}

// CollectionsClient defines the interface for Qdrant collections operations.
type CollectionsClient interface {
	Get(ctx context.Context, in *qdrant.GetCollectionInfoRequest, opts ...grpc.CallOption) (*qdrant.GetCollectionInfoResponse, error)
	Create(ctx context.Context, in *qdrant.CreateCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error)
	Delete(ctx context.Context, in *qdrant.DeleteCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error)
	List(ctx context.Context, in *qdrant.ListCollectionsRequest, opts ...grpc.CallOption) (*qdrant.ListCollectionsResponse, error)
}

// Options configures the Qdrant store.
type Options struct {
	// CollectionName is the default collection to use. Default: "documents"
	CollectionName string

	// Dimensions is the vector dimensionality. Required for auto-creation.
	Dimensions int

	// Metric specifies the distance metric. Default: Cosine
	Metric embedding.Metric

	// AutoCreateCollection creates the collection if it doesn't exist.
	AutoCreateCollection bool

	// GRPCDialOptions are additional options for the gRPC connection.
	GRPCDialOptions []grpc.DialOption
}

// Option configures a Store.
type Option func(*Options)

// WithCollectionName sets the default collection name.
func WithCollectionName(name string) Option {
	return func(o *Options) {
		o.CollectionName = name
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

// WithAutoCreateCollection enables automatic collection creation.
func WithAutoCreateCollection(auto bool) Option {
	return func(o *Options) {
		o.AutoCreateCollection = auto
	}
}

// Store is a Qdrant-backed VectorStore implementation.
type Store struct {
	client      PointsClient
	collections CollectionsClient
	conn        *grpc.ClientConn
	opts        Options
}

// New creates a new Qdrant vector store.
// conn is the gRPC connection (can be nil if Close won't be called).
// pointsClient and collectionsClient are the gRPC clients for Qdrant operations.
// Use qdrant.NewPointsClient(conn) and qdrant.NewCollectionsClient(conn) to create them,
// or pass mock implementations for testing.
func New(conn *grpc.ClientConn, pointsClient PointsClient, collectionsClient CollectionsClient, optFns ...Option) (*Store, error) {
	opts := Options{
		CollectionName:       "documents",
		Metric:               embedding.Cosine,
		AutoCreateCollection: true,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	store := &Store{
		client:      pointsClient,
		collections: collectionsClient,
		conn:        conn,
		opts:        opts,
	}

	// Auto-create collection if configured
	if opts.AutoCreateCollection && opts.Dimensions > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := store.ensureCollection(ctx, opts.CollectionName, opts.Dimensions, opts.Metric); err != nil {
			return nil, err
		}
	}

	return store, nil
}

// NewFromAddr creates a new Qdrant vector store by connecting to the given address.
// addr should be the gRPC endpoint (e.g., "localhost:6334").
func NewFromAddr(addr string, optFns ...Option) (*Store, error) {
	opts := Options{
		CollectionName:       "documents",
		Metric:               embedding.Cosine,
		AutoCreateCollection: true,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	dialOpts := opts.GRPCDialOptions
	if len(dialOpts) == 0 {
		dialOpts = []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
	}

	conn, err := grpc.NewClient(addr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}

	store, err := New(conn, qdrant.NewPointsClient(conn), qdrant.NewCollectionsClient(conn), optFns...)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return store, nil
}

// ensureCollection creates a collection if it doesn't exist.
func (s *Store) ensureCollection(ctx context.Context, name string, dims int, metric embedding.Metric) error {
	// Check if collection exists
	_, err := s.collections.Get(ctx, &qdrant.GetCollectionInfoRequest{
		CollectionName: name,
	})
	if err == nil {
		return nil // Collection exists
	}

	// Create collection
	distance := toQdrantDistance(metric)
	_, err = s.collections.Create(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: &qdrant.VectorsConfig{
			Config: &qdrant.VectorsConfig_Params{
				Params: &qdrant.VectorParams{
					Size:     safeconv.IntToUint64(dims),
					Distance: distance,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create collection %q: %w", name, err)
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

	collection := s.collectionName(opts.Namespace)

	// Auto-create collection if enabled
	if s.opts.AutoCreateCollection && s.opts.Dimensions > 0 {
		if err := s.ensureCollection(ctx, collection, s.opts.Dimensions, s.opts.Metric); err != nil {
			return err
		}
	}

	points := make([]*qdrant.PointStruct, len(docs))
	now := time.Now()

	for i, doc := range docs {
		// Generate or use provided ID
		docID := doc.ID
		if docID == "" {
			docID = uuid.New().String()
		}

		// Generate a deterministic UUID from the document ID for Qdrant
		pointUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(docID)).String()

		// Build payload
		payload := make(map[string]*qdrant.Value)
		payload["_id"] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: docID}} // Store original ID
		payload["content"] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: doc.Content}}

		ts := doc.Timestamp
		if ts.IsZero() {
			ts = now.Add(time.Duration(i) * time.Nanosecond)
		}
		payload["timestamp"] = &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: ts.UnixNano()}}

		// Add metadata
		for k, v := range doc.Metadata {
			payload[k] = toQdrantValue(v)
		}

		points[i] = &qdrant.PointStruct{
			Id:      &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: pointUUID}},
			Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vector{Vector: &qdrant.Vector{Data: floatconv.ToFloat32(doc.Embedding)}}},
			Payload: payload,
		}
	}

	wait := true
	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Points:         points,
		Wait:           &wait,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert points: %w", err)
	}

	return nil
}

// Search finds documents similar to the query embedding.
func (s *Store) Search(ctx context.Context, queryEmbedding embedding.Vector, opts vectorstore.SearchOptions) ([]vectorstore.Document, error) {
	opts.Normalize()
	collection := s.collectionName(opts.Namespace)

	// Build filter
	var filter *qdrant.Filter
	if len(opts.Filter) > 0 {
		filter = buildFilter(opts.Filter)
	}

	// Execute search
	resp, err := s.client.Search(ctx, &qdrant.SearchPoints{
		CollectionName: collection,
		Vector:         floatconv.ToFloat32(queryEmbedding),
		Limit:          safeconv.IntToUint64(opts.K),
		Filter:         filter,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: opts.IncludeEmbeddings}},
		ScoreThreshold: floatPtr(float32(opts.MinScore)),
	})
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert results
	results := make([]vectorstore.Document, 0, len(resp.Result))
	for _, point := range resp.Result {
		doc := extractDocument(point, opts.IncludeEmbeddings)
		results = append(results, doc)
	}

	return results, nil
}

// SearchHybrid performs a hybrid search combining dense vector similarity with sparse (keyword) search.
// This uses Qdrant's Query API with Reciprocal Rank Fusion (RRF) to combine results from both searches.
// Note: For keyword search, Qdrant requires a sparse vector field. If no sparse vectors are indexed,
// this method will perform a pure dense search with the query text used for filtering if a text field exists.
func (s *Store) SearchHybrid(ctx context.Context, query string, queryEmbedding embedding.Vector, opts vectorstore.HybridSearchOptions) ([]vectorstore.Document, error) {
	opts.Normalize()
	collection := s.collectionName(opts.Namespace)

	// Build filter
	var filter *qdrant.Filter
	if len(opts.Filter) > 0 {
		filter = buildFilter(opts.Filter)
	}

	// Determine fusion type based on options
	// RRF (0) = Reciprocal Rank Fusion, DBSF (1) = Distribution-Based Score Fusion
	fusion := qdrant.Fusion_RRF
	if opts.FusionAlgorithm == vectorstore.FusionRelativeScore {
		fusion = qdrant.Fusion(1) // DBSF - Distribution-Based Score Fusion
	}

	// Build prefetch queries for hybrid search
	// The alpha value determines the weight: 0.0 = pure keyword, 1.0 = pure vector
	prefetch := make([]*qdrant.PrefetchQuery, 0, 2)

	k := safeconv.IntToUint64(opts.K)
	prefetchLimit := k * 2 // Over-fetch for better fusion results

	// Dense vector search prefetch (always included unless alpha is 0)
	if opts.Alpha > 0 {
		prefetch = append(prefetch, &qdrant.PrefetchQuery{
			Query: qdrant.NewQueryDense(floatconv.ToFloat32(queryEmbedding)),
			Limit: &prefetchLimit,
		})
	}

	// Keyword/text search prefetch (if alpha < 1 and query is not empty)
	// This searches the content field for matching text
	if opts.Alpha < 1 && query != "" {
		// Use text match filter as a form of keyword search
		textFilter := &qdrant.Filter{
			Must: []*qdrant.Condition{
				{
					ConditionOneOf: &qdrant.Condition_Field{
						Field: &qdrant.FieldCondition{
							Key:   "content",
							Match: &qdrant.Match{MatchValue: &qdrant.Match_Text{Text: query}},
						},
					},
				},
			},
		}

		// If there's an existing filter, combine them
		combinedFilter := textFilter
		if filter != nil {
			combinedFilter = &qdrant.Filter{
				Must: append(textFilter.Must, filter.Must...),
			}
		}

		prefetch = append(prefetch, &qdrant.PrefetchQuery{
			Query:  qdrant.NewQueryDense(floatconv.ToFloat32(queryEmbedding)),
			Filter: combinedFilter,
			Limit:  &prefetchLimit,
		})
	}

	// If we only have one prefetch (either pure keyword or pure vector), just use regular search
	if len(prefetch) <= 1 {
		return s.Search(ctx, queryEmbedding, opts.SearchOptions)
	}

	// Execute hybrid query with fusion
	resp, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Prefetch:       prefetch,
		Query:          qdrant.NewQueryFusion(fusion),
		Limit:          &k,
		Filter:         filter,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: opts.IncludeEmbeddings}},
		ScoreThreshold: floatPtr(float32(opts.MinScore)),
	})
	if err != nil {
		return nil, fmt.Errorf("hybrid search failed: %w", err)
	}

	// Convert results
	results := make([]vectorstore.Document, 0, len(resp.Result))
	for _, point := range resp.Result {
		doc := extractDocument(point, opts.IncludeEmbeddings)
		results = append(results, doc)
	}

	return results, nil
}

// Delete removes documents by ID.
func (s *Store) Delete(ctx context.Context, ids []string, namespace string) error {
	if len(ids) == 0 {
		return nil
	}

	collection := s.collectionName(namespace)
	pointIDs := make([]*qdrant.PointId, len(ids))
	for i, id := range ids {
		// Convert document ID to Qdrant UUID using the same mapping as Add
		pointUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String()
		pointIDs[i] = &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: pointUUID}}
	}

	wait := true
	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Points:         &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Points{Points: &qdrant.PointsIdsList{Ids: pointIDs}}},
		Wait:           &wait,
	})
	if err != nil {
		return fmt.Errorf("failed to delete points: %w", err)
	}

	return nil
}

// Close releases resources.
func (s *Store) Close() error {
	return s.conn.Close()
}

// CreateIndex creates a new collection.
func (s *Store) CreateIndex(ctx context.Context, name string, dims int, metric embedding.Metric) error {
	distance := toQdrantDistance(metric)
	_, err := s.collections.Create(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: &qdrant.VectorsConfig{
			Config: &qdrant.VectorsConfig_Params{
				Params: &qdrant.VectorParams{
					Size:     safeconv.IntToUint64(dims),
					Distance: distance,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create collection %q: %w", name, err)
	}
	return nil
}

// DeleteIndex removes a collection and all its data.
func (s *Store) DeleteIndex(ctx context.Context, name string) error {
	_, err := s.collections.Delete(ctx, &qdrant.DeleteCollection{
		CollectionName: name,
	})
	if err != nil {
		return fmt.Errorf("failed to delete collection %q: %w", name, err)
	}
	return nil
}

// ListIndexes returns all available collections.
func (s *Store) ListIndexes(ctx context.Context) ([]string, error) {
	resp, err := s.collections.List(ctx, &qdrant.ListCollectionsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	names := make([]string, len(resp.Collections))
	for i, c := range resp.Collections {
		names[i] = c.Name
	}
	return names, nil
}

// collectionName returns the collection name, using namespace as suffix if provided.
func (s *Store) collectionName(namespace string) string {
	if namespace == "" {
		return s.opts.CollectionName
	}
	return s.opts.CollectionName + "_" + namespace
}

// extractDocument converts a Qdrant ScoredPoint to a Document.
func extractDocument(point *qdrant.ScoredPoint, includeEmbeddings bool) vectorstore.Document {
	doc := vectorstore.Document{
		Score: float64(point.Score),
	}

	if point.Payload == nil {
		return doc
	}

	// Get original document ID from payload
	if v, ok := point.Payload["_id"]; ok {
		doc.ID = v.GetStringValue()
	} else {
		// Fallback to Qdrant point ID
		doc.ID = extractID(point.Id)
	}

	if v, ok := point.Payload["content"]; ok {
		doc.Content = v.GetStringValue()
	}

	if v, ok := point.Payload["timestamp"]; ok {
		doc.Timestamp = time.Unix(0, v.GetIntegerValue())
	}

	// Extract metadata (exclude internal fields)
	doc.Metadata = make(map[string]any)
	for k, v := range point.Payload {
		if k != "_id" && k != "content" && k != "timestamp" {
			doc.Metadata[k] = fromQdrantValue(v)
		}
	}

	// Extract embedding if requested
	if includeEmbeddings && point.Vectors != nil {
		doc.Embedding = extractEmbedding(point.Vectors)
	}

	return doc
}

// extractEmbedding extracts the vector data from VectorsOutput.
func extractEmbedding(vectors *qdrant.VectorsOutput) []float64 {
	if vectors == nil {
		return nil
	}

	vec := vectors.GetVector()
	if vec == nil {
		return nil
	}

	// Try the new Dense field first
	if dense := vec.GetDense(); dense != nil && len(dense.GetData()) > 0 {
		return floatconv.ToFloat64(dense.GetData())
	}

	// Fall back to deprecated Data field for older Qdrant versions
	//nolint:staticcheck // Support older Qdrant versions
	if len(vec.GetData()) > 0 {
		return floatconv.ToFloat64(vec.GetData())
	}

	return nil
}

// Helper functions

func toQdrantDistance(metric embedding.Metric) qdrant.Distance {
	switch metric {
	case embedding.Euclidean:
		return qdrant.Distance_Euclid
	case embedding.DotProduct:
		return qdrant.Distance_Dot
	default:
		return qdrant.Distance_Cosine
	}
}

func toQdrantValue(v any) *qdrant.Value {
	switch val := v.(type) {
	case string:
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: val}}
	case int:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(val)}}
	case int64:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: val}}
	case float64:
		return &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: val}}
	case bool:
		return &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: val}}
	default:
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: fmt.Sprintf("%v", v)}}
	}
}

func fromQdrantValue(v *qdrant.Value) any {
	switch k := v.Kind.(type) {
	case *qdrant.Value_StringValue:
		return k.StringValue
	case *qdrant.Value_IntegerValue:
		return k.IntegerValue
	case *qdrant.Value_DoubleValue:
		return k.DoubleValue
	case *qdrant.Value_BoolValue:
		return k.BoolValue
	default:
		return nil
	}
}

func extractID(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	switch v := id.PointIdOptions.(type) {
	case *qdrant.PointId_Uuid:
		return v.Uuid
	case *qdrant.PointId_Num:
		return fmt.Sprintf("%d", v.Num)
	default:
		return ""
	}
}

func buildFilter(filter vectorstore.Filter) *qdrant.Filter {
	must := make([]*qdrant.Condition, 0, len(filter))
	for k, v := range filter {
		must = append(must, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key:   k,
					Match: &qdrant.Match{MatchValue: &qdrant.Match_Keyword{Keyword: fmt.Sprintf("%v", v)}},
				},
			},
		})
	}
	return &qdrant.Filter{Must: must}
}

func floatPtr(f float32) *float32 {
	return &f
}
