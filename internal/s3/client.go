package s3

import (
	"context"
	"log"

	"github.com/MediSynth-io/medisynth/internal/config"
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
