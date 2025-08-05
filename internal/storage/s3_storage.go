package storage

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/paragor/sharefile/internal/log"
)

type s3StorageFactory struct {
	client *s3.S3
	bucket string
}

func NewS3Storage(client *s3.S3, bucket string) Storage {
	return &s3StorageFactory{client: client, bucket: bucket}
}

func (sf *s3StorageFactory) OpenStorage(ctx context.Context, email string, autoCreate bool) (UserScopedStorage, error) {
	if email == "" {
		return nil, fmt.Errorf("email cannot be empty")
	}
	err := sf.createMetadataIfNotExists(ctx, email, autoCreate)
	if err != nil {
		return nil, fmt.Errorf("cant open s3 storage: %w", err)
	}

	return &s3SUserSCopedStorage{
		client:   sf.client,
		uploader: s3manager.NewUploaderWithClient(sf.client),
		bucket:   sf.bucket,
		email:    email,
	}, nil
}

func (sf *s3StorageFactory) createMetadataIfNotExists(ctx context.Context, email string, autoCreate bool) error {
	key := email + "/" + metadataFile
	obj, err := sf.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Key:    aws.String(key),
		Bucket: aws.String(sf.bucket),
	})

	if err == nil {
		defer obj.Body.Close()
		meta, err := readMetadata(obj.Body)
		if err != nil {
			return fmt.Errorf("cant read metadata: %w", err)
		}
		if meta.Email != email {
			return fmt.Errorf("invalid metadata: expect %s email, got %s", email, meta.Email)
		}

		if meta.MigrationRequired() {
			log.FromContext(ctx).Info("migrate metadata")
			meta.Migrate()
			data, err := meta.marshal()
			if err != nil {
				return fmt.Errorf("unexpected marshal error during migration: %w", err)
			}
			if _, err := sf.client.PutObjectWithContext(ctx, &s3.PutObjectInput{
				Key:         aws.String(key),
				Body:        bytes.NewReader(data),
				Bucket:      aws.String(sf.bucket),
				ContentType: aws.String("application/json"),
			}); err != nil {
				return fmt.Errorf("cant upload migrated metadata: %w", err)
			}
		}
		return nil
	}

	if awsErr, ok := err.(awserr.Error); ok && autoCreate && awsErr.Code() == s3.ErrCodeNoSuchKey {
		log.FromContext(ctx).Info("create new metadata")
		data, err := newMetadata(email).marshal()
		if err != nil {
			return fmt.Errorf("unexpected marshal error: %w", err)
		}
		if _, err := sf.client.PutObjectWithContext(ctx, &s3.PutObjectInput{
			Key:         aws.String(key),
			Body:        bytes.NewReader(data),
			Bucket:      aws.String(sf.bucket),
			ContentType: aws.String("application/json"),
		}); err != nil {
			return fmt.Errorf("cant upload empty metadata: %w", err)
		}
		return nil
	}

	return fmt.Errorf("cant read meatadata from s3: %w", err)
}
