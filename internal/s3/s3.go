package s3

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Client represents an S3 client
type Client struct {
	client     *s3.Client
	bucketName string
}

// NewClient creates a new S3 client
func NewClient(endpoint, region, accessKey, secretKey, bucketName string) (*Client, error) {
	// Create custom endpoint resolver
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: endpoint,
		}, nil
	})

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	client := s3.NewFromConfig(cfg)

	return &Client{
		client:     client,
		bucketName: bucketName,
	}, nil
}

// UploadFile uploads a file to S3
func (c *Client) UploadFile(ctx context.Context, key string, body io.Reader) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
		Body:   body,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}
	return nil
}

// GetFileURL generates a presigned URL for downloading a file
func (c *Client) GetFileURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(c.client)
	presignedURL, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presignedURL.URL, nil
}

// ListFiles lists files in a directory
func (c *Client) ListFiles(ctx context.Context, prefix string) ([]string, error) {
	result, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucketName),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list files in S3: %w", err)
	}

	var files []string
	for _, obj := range result.Contents {
		files = append(files, *obj.Key)
	}
	return files, nil
}

// GetFileSize gets the size of a file
func (c *Client) GetFileSize(ctx context.Context, key string) (int64, error) {
	result, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get file size from S3: %w", err)
	}
	return result.ContentLength, nil
}

// GenerateJobPath generates the S3 path for a job's output files
func GenerateJobPath(userID, jobID string) string {
	return path.Join("jobs", userID, jobID)
}

// GenerateJobFilePath generates the S3 path for a specific job output file
func GenerateJobFilePath(userID, jobID, filename string) string {
	return path.Join(GenerateJobPath(userID, jobID), filename)
}
