// server/internal/s3/uploader.go
package s3

import (
	"context"
	"fmt"
	"io"
	"fresh-meat-scm-api-server/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Uploader struct {
	Client *s3.Client
	Bucket          string
	Region          string
	CloudFrontDomain string
}

func NewUploader(cfg config.S3Config) (*Uploader, error) {
	sdkConfig, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(sdkConfig)

	return &Uploader{
		Client: s3Client,
		Bucket:          cfg.Bucket,
		Region:          cfg.Region,
		CloudFrontDomain: cfg.CloudFrontDomain,
	}, nil
}

// UploadFile uploads a file to S3 and returns its URL.
func (u *Uploader) UploadFile(ctx context.Context, file io.Reader, objectKey string) (string, error) {
	_, err := u.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(u.Bucket),
		Key:    aws.String(objectKey),
		Body:   file,
		ContentType: aws.String("image/jpeg"), // Giả sử tất cả ảnh đều là JPEG\\\
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file to S3: %w", err)
	}

	if u.CloudFrontDomain != "" {
		// Nếu có, xây dựng URL CloudFront
		url := fmt.Sprintf("https://%s/%s", u.CloudFrontDomain, objectKey)
		return url, nil
	}

	// Nếu không, quay trở lại xây dựng URL S3 (fallback)
	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", u.Bucket, u.Region, objectKey)
	return url, nil
}