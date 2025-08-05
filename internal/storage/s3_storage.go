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

func getS3MetadataPath(email string) string {
	return email + "/" + metadataFile
}

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
	meta, err := sf.openMetadata(ctx, email, autoCreate)
	if err != nil {
		return nil, fmt.Errorf("cant open metadata: %w", err)
	}
	if err := sf.migrateMetadata(ctx, meta); err != nil {
		return nil, fmt.Errorf("cant migrate metadata: %w", err)
	}

	return &s3SUserSCopedStorage{
		client:   sf.client,
		uploader: s3manager.NewUploaderWithClient(sf.client),
		bucket:   sf.bucket,
		email:    meta.Email,
	}, nil
}

func (sf *s3StorageFactory) saveMetadata(ctx context.Context, meta *Metadata) error {
	data, err := meta.marshal()
	if err != nil {
		return fmt.Errorf("cant marshal metadata: %w", err)
	}
	if _, err := sf.client.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Key:         aws.String(getS3MetadataPath(meta.Email)),
		Body:        bytes.NewReader(data),
		Bucket:      aws.String(sf.bucket),
		ContentType: aws.String("application/json"),
	}); err != nil {
		return fmt.Errorf("cant upload metadata: %w", err)
	}
	return nil
}

func (sf *s3StorageFactory) migrateMetadata(ctx context.Context, meta *Metadata) error {
	if !meta.MigrationRequired() {
		return nil
	}
	log.FromContext(ctx).Info("migrate metadata")
	meta.Migrate()
	if err := sf.saveMetadata(ctx, meta); err != nil {
		return fmt.Errorf("cant upload migrated metadata: %w", err)
	}
	return nil
}

func (sf *s3StorageFactory) openMetadata(ctx context.Context, email string, autoCreate bool) (*Metadata, error) {
	obj, err := sf.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Key:    aws.String(getS3MetadataPath(email)),
		Bucket: aws.String(sf.bucket),
	})

	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && autoCreate && awsErr.Code() == s3.ErrCodeNoSuchKey {
			log.FromContext(ctx).Info("create new metadata")
			meta := newMetadata(email)
			if err := sf.saveMetadata(ctx, meta); err != nil {
				return nil, fmt.Errorf("cant upload new metadata: %w", err)
			}
			return meta, nil
		}
		return nil, fmt.Errorf("cant read meatadata from s3: %w", err)
	}
	defer obj.Body.Close()

	meta, err := readMetadata(obj.Body)
	if err != nil {
		return nil, fmt.Errorf("cant read metadata: %w", err)
	}
	if meta.Email != email {
		return nil, fmt.Errorf("invalid metadata: expect %s email, got %s", email, meta.Email)
	}
	return meta, nil
}
