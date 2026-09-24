package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrInvalidKey = errors.New("storage key is invalid")
	ErrNotFound   = errors.New("storage object was not found")
	ErrReadOnly   = errors.New("local storage does not support signed URLs")
)

type LocalStore struct {
	Root string
}

func NewLocal(root string) (*LocalStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalidKey
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	return &LocalStore{Root: resolved}, nil
}

func (s *LocalStore) Put(ctx context.Context, key string, body io.Reader, contentType string) (Object, error) {
	path, err := s.path(key)
	if err != nil {
		return Object{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Object{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".dawha-upload-*")
	if err != nil {
		return Object{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := copyWithContext(ctx, temporary, body); err != nil {
		_ = temporary.Close()
		return Object{}, err
	}
	if err := temporary.Close(); err != nil {
		return Object{}, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return Object{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Object{}, err
	}
	return Object{Key: key, ContentType: contentType, Size: info.Size()}, nil
}

func (s *LocalStore) Get(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, Object{}, err
	}
	path, err := s.path(key)
	if err != nil {
		return nil, Object{}, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, Object{}, ErrNotFound
	}
	if err != nil {
		return nil, Object{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, Object{}, err
	}
	return file, Object{Key: key, Size: info.Size()}, nil
}

func (s *LocalStore) SignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", ErrReadOnly
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *LocalStore) path(key string) (string, error) {
	if s == nil || strings.TrimSpace(s.Root) == "" || strings.TrimSpace(key) == "" || filepath.IsAbs(key) {
		return "", ErrInvalidKey
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrInvalidKey
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", ErrInvalidKey
	}
	if err := rejectSymlinkComponents(root, relative); err != nil {
		return "", err
	}
	return path, nil
}

func rejectSymlinkComponents(root, relative string) error {
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidKey
		}
	}
	return nil
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) error {
	buffer := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			if _, err := destination.Write(buffer[:read]); err != nil {
				return fmt.Errorf("write storage object: %w", err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
