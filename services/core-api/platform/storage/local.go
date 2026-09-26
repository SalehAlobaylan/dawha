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

// LocalStore is the DEVELOPMENT adapter: bytes in a directory.
//
// It is an explicit adapter, not the absence of one. `npm run dev`, the CI
// database job and the browser suite all run against it, and the production
// adapter is chosen by configuration rather than by which process happens to be
// running. What it is NOT is a fallback: nothing silently degrades to it.
type LocalStore struct {
	Root string
	// signer, when set, is what SignedURL mints with. Without it the store holds
	// and serves bytes and refuses to mint links, which is a different thing from
	// not having a store at all.
	signer *LocalSigner
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

// WithSigner returns the same store, able to mint signed URLs that point back at
// this service's object route. It is a separate call rather than a constructor
// argument because most callers of a local store only ever Put and Get, and
// making them supply a secret they do not use would be a worse trade than one
// line here.
func (s *LocalStore) WithSigner(signer *LocalSigner) *LocalStore {
	if s == nil {
		return nil
	}
	s.signer = signer
	return s
}

// Signer exposes the configured signer, which the object route needs in order to
// verify what SignedURL minted.
func (s *LocalStore) Signer() *LocalSigner {
	if s == nil {
		return nil
	}
	return s.signer
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

// SignedURL mints an expiring, HMAC-signed link to this service's own object
// route. It used to return ErrReadOnly: "the development adapter cannot do signed
// access" was true of the old runner and stopped being true here, and an adapter
// that refuses is an adapter the local stack quietly routes around.
func (s *LocalStore) SignedURL(_ context.Context, key string, expires time.Duration) (string, error) {
	if s == nil {
		return "", ErrNotFound
	}
	if s.signer == nil {
		return "", ErrSignerNotConfigured
	}
	return s.signer.Sign(key, expires)
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
	if s == nil || strings.TrimSpace(s.Root) == "" {
		return "", ErrInvalidKey
	}
	// The shared key rule first, so a key this store refuses is a key the S3
	// adapter refuses too and the two cannot drift apart on what a key may be.
	if !ValidKey(key) {
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
