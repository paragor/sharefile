package storage

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type s3SUserSCopedStorage struct {
	client   *s3.S3
	uploader *s3manager.Uploader
	bucket   string
	email    string
}

func (s *s3SUserSCopedStorage) getFilePath(objPath string) string {
	return s.email + "/files/" + strings.TrimLeft(objPath, "/")
}

func (s *s3SUserSCopedStorage) GetMetadata(ctx context.Context) (*Metadata, error) {
	obj, err := s.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Key:    aws.String(getS3MetadataPath(s.email)),
		Bucket: aws.String(s.bucket),
	})

	if err != nil {
		return nil, fmt.Errorf("cant read metadata from s3: %w", err)
	}
	defer obj.Body.Close()
	meta, err := readMetadata(obj.Body)
	if err != nil {
		return nil, fmt.Errorf("cant read metadata: %w", err)
	}
	return meta, nil
}

func (s *s3SUserSCopedStorage) Upload(ctx context.Context, objPath string, contentType string, file io.Reader) error {
	_, err := s.uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Body:        file,
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.getFilePath(objPath)),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("cant upload file: %w", err)
	}
	return nil
}

func (s *s3SUserSCopedStorage) Delete(ctx context.Context, objPath string) error {
	_, err := s.client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.getFilePath(objPath)),
	})
	if err != nil {
		return fmt.Errorf("cant delete file: %w", err)
	}
	return nil
}

func (s *s3SUserSCopedStorage) GenerateDownloadLink(ctx context.Context, objPath string, expiration time.Duration) (string, error) {
	req, _ := s.client.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.getFilePath(objPath)),
	})
	urlStr, err := req.Presign(expiration)
	if err != nil {
		return "", fmt.Errorf("cant presign url: %w", err)
	}

	return urlStr, nil
}

func (s *s3SUserSCopedStorage) ListFiles(ctx context.Context) ([]FileInList, error) {
	output, err := s.client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.email + "/files/"),
	})
	if err != nil {
		return nil, fmt.Errorf("cant list s3 files: %w", err)
	}

	listing := make([]FileInList, 0, len(output.Contents))
	for _, obj := range output.Contents {
		listing = append(listing, FileInList{
			Path:           strings.TrimPrefix(*obj.Key, s.email+"/files/"),
			LastModifiedAt: *obj.LastModified,
			Size:           int(*obj.Size),
		})
	}
	sort.SliceStable(listing, func(i, j int) bool {
		return listing[j].LastModifiedAt.Before(listing[i].LastModifiedAt)
	})
	return listing, nil
}

func (s *s3SUserSCopedStorage) Move(ctx context.Context, objPathOld string, objPathNew string) error {
	_, err := s.client.CopyObjectWithContext(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(s.bucket + "/" + s.getFilePath(objPathOld)),
		Key:        aws.String(s.getFilePath(objPathNew)),
	})
	if err != nil {
		return fmt.Errorf("cant copy s3 file: %w", err)
	}

	_, err = s.client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.getFilePath(objPathOld)),
	})
	if err != nil {
		return fmt.Errorf("cant delete file: %w", err)
	}
	return nil

}
