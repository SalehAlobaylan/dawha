package storage

import (
	"context"
	"io"
	"time"
)

type Object struct {
	Key         string
	ContentType string
	Size        int64
}

type Store interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) (Object, error)
	SignedURL(ctx context.Context, key string, expires time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
}
