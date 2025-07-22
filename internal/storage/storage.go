package storage

import (
	"context"
	"io"
	"time"
)

type UserScopedStorage interface {
	GetMetadata(ctx context.Context) (*Metadata, error)
	Upload(ctx context.Context, objPath string, contentType string, file io.Reader) error
	Move(ctx context.Context, objPathOld string, objPathNew string) error
	Delete(ctx context.Context, objPath string) error
	GenerateDownloadLink(ctx context.Context, objPath string, expiration time.Duration) (string, error)
	// ListFiles return list of objects, sorted by last modified desc
	ListFiles(ctx context.Context) ([]FileInList, error)
}

type Storage interface {
	OpenStorage(ctx context.Context, email string, autoCreate bool) (UserScopedStorage, error)
}
