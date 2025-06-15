package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Client struct {
	client    *s3.Client
	bucket    string
	cdnDomain string // DigitalOcean CDN domain for faster downloads
}

type UploadResult struct {
	Key      string
	URL      string
	Size     int64
	Checksum string
}

// NewS3Client creates a new S3 client configured for DigitalOcean Spaces
func NewS3Client(endpoint, region, bucket, accessKeyID, secretAccessKey string, useSSL bool) (*S3Client, error) {
	// Generate CDN domain from bucket and region
	cdnDomain := fmt.Sprintf("https://%s.%s.cdn.digitaloceanspaces.com", bucket, region)
	// Configure custom resolver for DigitalOcean Spaces
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{
				URL:           endpoint,
				SigningRegion: region,
			}, nil
		}
		return aws.Endpoint{}, fmt.Errorf("unknown endpoint requested")
	})

	// Load config with custom credentials and endpoint
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyID, secretAccessKey, "",
		)),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	// Create S3 client
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true // Required for DigitalOcean Spaces
	})

	return &S3Client{
		client:    client,
		bucket:    bucket,
		cdnDomain: cdnDomain,
	}, nil
}

// UploadFile uploads a file to S3 and returns the result
func (s *S3Client) UploadFile(ctx context.Context, key string, reader io.Reader, contentType string) (*UploadResult, error) {
	// Upload the file
	putInput := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String(contentType),
		ACL:         types.ObjectCannedACLPrivate, // Private by default
	}

	result, err := s.client.PutObject(ctx, putInput)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	// Get object info for size
	headInput := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	headOutput, err := s.client.HeadObject(ctx, headInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get object info: %w", err)
	}

	var size int64
	if headOutput.ContentLength != nil {
		size = *headOutput.ContentLength
	}

	return &UploadResult{
		Key:      key,
		URL:      fmt.Sprintf("%s/%s", s.cdnDomain, key), // Use CDN URL for faster downloads
		Size:     size,
		Checksum: aws.ToString(result.ETag),
	}, nil
}

// GeneratePresignedURL creates a presigned URL for downloading a file
func (s *S3Client) GeneratePresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.client)

	getInput := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	url, err := presignClient.PresignGetObject(ctx, getInput, func(opts *s3.PresignOptions) {
		opts.Expires = expiration
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return url.URL, nil
}

// UploadJobOutput uploads Synthea job output files to S3
func (s *S3Client) UploadJobOutput(ctx context.Context, userID, jobID string, files map[string]io.Reader) ([]*UploadResult, error) {
	var results []*UploadResult

	for filename, reader := range files {
		// Generate S3 key: users/{userID}/jobs/{jobID}/{filename}
		key := fmt.Sprintf("users/%s/jobs/%s/%s", userID, jobID, filename)

		// Determine content type based on file extension
		contentType := getContentType(filename)

		result, err := s.UploadFile(ctx, key, reader, contentType)
		if err != nil {
			return nil, fmt.Errorf("failed to upload %s: %w", filename, err)
		}

		results = append(results, result)
	}

	return results, nil
}

// getContentType returns the appropriate content type based on file extension
func getContentType(filename string) string {
	ext := filepath.Ext(filename)
	switch ext {
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".csv":
		return "text/csv"
	case ".zip":
		return "application/zip"
	case ".tar.gz", ".tgz":
		return "application/gzip"
	default:
		return "application/octet-stream"
	}
}

// ListUserJobs lists all job outputs for a user
func (s *S3Client) ListUserJobs(ctx context.Context, userID string) ([]string, error) {
	prefix := fmt.Sprintf("users/%s/jobs/", userID)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	}

	result, err := s.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	var keys []string
	for _, obj := range result.Contents {
		keys = append(keys, aws.ToString(obj.Key))
	}

	return keys, nil
}

// DeleteJobOutput deletes all files for a specific job
func (s *S3Client) DeleteJobOutput(ctx context.Context, userID, jobID string) error {
	prefix := fmt.Sprintf("users/%s/jobs/%s/", userID, jobID)

	// List objects to delete
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	}

	listResult, err := s.client.ListObjectsV2(ctx, listInput)
	if err != nil {
		return fmt.Errorf("failed to list objects for deletion: %w", err)
	}

	if len(listResult.Contents) == 0 {
		return nil // Nothing to delete
	}

	// Prepare objects for deletion
	var objectsToDelete []types.ObjectIdentifier
	for _, obj := range listResult.Contents {
		objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
			Key: obj.Key,
		})
	}

	// Delete objects
	deleteInput := &s3.DeleteObjectsInput{
		Bucket: aws.String(s.bucket),
		Delete: &types.Delete{
			Objects: objectsToDelete,
		},
	}

	_, err = s.client.DeleteObjects(ctx, deleteInput)
	if err != nil {
		return fmt.Errorf("failed to delete objects: %w", err)
	}

	return nil
}
