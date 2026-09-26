package storage

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
)

// DriverLocal and DriverS3 are the two adapters, named by configuration.
const (
	DriverLocal = "local"
	DriverS3    = "s3"
)

// Config is the whole storage configuration, read from the environment in one
// place so that the API, the source-processing worker, the analysis worker and
// the contradiction detector cannot disagree about which store they are using.
//
// They used to: each of them called storage.NewLocal itself, which meant a
// deployment that configured a bucket still had three processes writing to a
// directory, and nothing anywhere said so.
type Config struct {
	// Driver is "local" (the default) or "s3". The default is local because the
	// documented local stack - `npm run dev`, the CI database job, `make e2e` -
	// must work with no credentials and no bucket. A deployment selects s3
	// explicitly; nothing infers it from the presence of an environment variable.
	Driver string
	// LocalRoot is the directory LocalStore writes to.
	LocalRoot string
	// SigningBaseURL is the externally reachable base URL of this service, which
	// is what a locally signed object URL points back at. Required for signed
	// access with the local driver; without it the local store still holds and
	// serves bytes, and refuses to mint links.
	SigningBaseURL string
	// SigningSecret signs local object URLs. Empty means "generate one for this
	// process", which is the right default for development - every restart
	// invalidates every link somebody copied out of a terminal - and the wrong
	// one for a deployment, which must set it so a signed link survives a
	// rolling restart.
	SigningSecret []byte
	// S3 carries the production adapter's configuration.
	S3 S3Config
}

// ConfigFromEnvironment reads the storage configuration.
//
// The variables, and only the variables:
//
//	STORAGE_DRIVER            local (default) | s3
//	SOURCE_STORAGE_DIR        the local adapter's directory
//	STORAGE_SIGNING_BASE_URL  the API's external base URL, for local signed URLs
//	STORAGE_SIGNING_SECRET    the local signing secret; generated if unset
//	S3_BUCKET                 required for the s3 driver
//	S3_REGION                 required for the s3 driver
//	S3_ENDPOINT               optional; empty means AWS
//	S3_ACCESS_KEY_ID          optional; both halves or neither
//	S3_SECRET_ACCESS_KEY      optional
//	S3_USE_PATH_STYLE         optional; required by most S3-compatible servers
//
// No credential is read from a file and no credential is logged. The names are
// here; the values stay in the process that needs them.
func ConfigFromEnvironment(getenv func(string) string) Config {
	read := func(name string) string {
		if getenv == nil {
			return ""
		}
		return strings.TrimSpace(getenv(name))
	}
	config := Config{
		Driver:         read("STORAGE_DRIVER"),
		LocalRoot:      read("SOURCE_STORAGE_DIR"),
		SigningBaseURL: read("STORAGE_SIGNING_BASE_URL"),
		S3: S3Config{
			Bucket:          read("S3_BUCKET"),
			Region:          read("S3_REGION"),
			Endpoint:        read("S3_ENDPOINT"),
			AccessKeyID:     read("S3_ACCESS_KEY_ID"),
			SecretAccessKey: read("S3_SECRET_ACCESS_KEY"),
			UsePathStyle:    truthy(read("S3_USE_PATH_STYLE")),
		},
	}
	if secret := read("STORAGE_SIGNING_SECRET"); secret != "" {
		config.SigningSecret = []byte(secret)
	}
	return config
}

// New resolves the configuration into a store, and fails closed.
//
// "Fails closed" is the whole contract. Selecting the s3 driver without a bucket
// is a startup error, not a store that quietly writes to a directory; selecting
// a driver that does not exist is a startup error, not a nil store that panics on
// the first upload.
func New(ctx context.Context, config Config) (Store, error) {
	driver := strings.ToLower(strings.TrimSpace(config.Driver))
	if driver == "" {
		driver = DriverLocal
	}
	switch driver {
	case DriverLocal:
		root := config.LocalRoot
		if strings.TrimSpace(root) == "" {
			return nil, fmt.Errorf("%w: the local driver needs SOURCE_STORAGE_DIR", ErrNotConfigured)
		}
		store, err := NewLocal(root)
		if err != nil {
			return nil, err
		}
		// A local store without a signing base URL is a legitimate
		// configuration: it holds and serves bytes and cannot mint links. That
		// is a different thing from refusing to start.
		if strings.TrimSpace(config.SigningBaseURL) == "" {
			return store, nil
		}
		secret := config.SigningSecret
		if len(secret) == 0 {
			generated, err := randomSecret()
			if err != nil {
				return nil, err
			}
			secret = generated
		}
		signer, err := NewLocalSigner(config.SigningBaseURL, secret)
		if err != nil {
			return nil, err
		}
		return store.WithSigner(signer), nil
	case DriverS3:
		return NewS3(ctx, config.S3)
	default:
		return nil, fmt.Errorf("%w: STORAGE_DRIVER=%q is not a driver; use %q or %q", ErrNotConfigured, config.Driver, DriverLocal, DriverS3)
	}
}

// LocalStoreFor returns the underlying local store when the store is one, and
// false otherwise. The object route needs it to reach the signer, and asking for
// a concrete type is better than putting a Verify method on the Store interface
// that the S3 adapter has no use for.
func LocalStoreFor(store Store) (*LocalStore, bool) {
	local, ok := store.(*LocalStore)
	return local, ok
}

// randomSecret is the development default for STORAGE_SIGNING_SECRET.
//
// crypto/rand failing is not something to paper over with a constant: a
// predictable signing secret is worse than no signed access at all, so the
// failure propagates and the process does not start with one.
func randomSecret() ([]byte, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("%w: a signing secret could not be generated: %v", ErrNotConfigured, err)
	}
	return secret, nil
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
