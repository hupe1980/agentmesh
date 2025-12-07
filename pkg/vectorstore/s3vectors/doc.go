// Package s3vectors provides an Amazon S3 Vectors-backed VectorStore implementation.
//
// # Overview
//
// This package implements the vectorstore.VectorStore and vectorstore.Indexer
// interfaces using Amazon S3 Vectors as the backend. S3 Vectors is a purpose-built
// vector storage service that provides cost-optimized storage and querying at scale.
//
// # Features
//
//   - Native AWS integration with IAM authentication
//   - Cost-optimized vector storage (up to 90% cost reduction)
//   - Sub-second query latency
//   - Scales to billions of vectors
//   - Metadata filtering support
//
// # Usage
//
//	store, err := s3vectors.New(ctx,
//	    s3vectors.WithVectorBucketName("my-bucket"),
//	    s3vectors.WithIndexName("my-index"),
//	)
//	defer store.Close()
//
// # AWS Configuration
//
// Uses the default AWS credential chain (env vars, credentials file, IAM role).
package s3vectors
