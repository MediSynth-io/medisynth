package s3

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Client wraps the S3 client
type Client struct {
	*s3.Client
	BucketName string
}

// NewClient creates and configures a new S3 client
func NewClient(cfg *config.Config) (*Client, error) {
	log.Println("Initializing S3 client...")

	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:           cfg.S3Endpoint,
			SigningRegion: cfg.S3Region,
		}, nil
	})

	awsCfg, err := awsConfig.LoadDefaultConfig(context.TODO(),
		awsConfig.WithEndpointResolverWithOptions(resolver),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.S3AccessKeyID, cfg.S3SecretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg)

	log.Printf("S3 client initialized for bucket: %s, region: %s", cfg.S3Bucket, cfg.S3Region)

	return &Client{
		Client:     client,
		BucketName: cfg.S3Bucket,
	}, nil
}

func (c *Client) ListFiles(ctx context.Context, prefix string) ([]models.JobFile, error) {
	output, err := c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &c.BucketName,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, err
	}

	presignClient := s3.NewPresignClient(c.Client)
	var files []models.JobFile

	for _, object := range output.Contents {
		req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: &c.BucketName,
			Key:    object.Key,
		}, func(opts *s3.PresignOptions) {
			opts.Expires = 24 * time.Hour
		})
		if err != nil {
			log.Printf("Failed to generate presigned URL for key %s: %v", *object.Key, err)
			continue // Or handle error differently
		}

		var size int64
		if object.Size != nil {
			size = *object.Size
		}

		files = append(files, models.JobFile{
			Name:      extractFilename(*object.Key),
			Size:      size,
			Timestamp: *object.LastModified,
			URL:       req.URL,
		})
	}

	return files, nil
}

func extractFilename(s3Key string) string {
	// Extract just the filename from the S3 key path
	// For example: "synthea_output/job-123/fhir/Patient_123.json" -> "Patient_123.json"
	filename := filepath.Base(s3Key)

	// If it's still empty or just a path separator, use the last meaningful part
	if filename == "." || filename == "/" || filename == "" {
		parts := strings.Split(strings.Trim(s3Key, "/"), "/")
		if len(parts) > 0 {
			filename = parts[len(parts)-1]
		}
	}

	// If we still don't have a good filename, use the full key
	if filename == "" {
		filename = s3Key
	}

	return filename
}

// ListJobFiles lists all files for a specific job ID
func (c *Client) ListJobFiles(jobID string) ([]models.JobFile, error) {
	prefix := "synthea_output/" + jobID + "/"
	return c.ListFiles(context.TODO(), prefix)
}

// UploadFile uploads a file to S3
func (c *Client) UploadFile(ctx context.Context, key string, body io.Reader) error {
	_, err := c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &c.BucketName,
		Key:    &key,
		Body:   body,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}
	return nil
}

// GetFileSize gets the size of a file in S3
func (c *Client) GetFileSize(ctx context.Context, key string) (int64, error) {
	result, err := c.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &c.BucketName,
		Key:    &key,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get file size from S3: %w", err)
	}
	return result.ContentLength, nil
}

// GetFileURL generates a presigned URL for downloading a file
func (c *Client) GetFileURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(c.Client)
	presignedURL, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &c.BucketName,
		Key:    &key,
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presignedURL.URL, nil
}
