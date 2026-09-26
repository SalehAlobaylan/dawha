package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Store is the PRODUCTION adapter: bytes in an S3-compatible bucket, download
// access by presigned URL.
//
// It is built on the AWS SDK for Go v2 rather than on a hand-written SigV4
// signer. That is the whole reason the SDK is a dependency: presigning is a
// signing algorithm over a canonical request with four AWS-specific escaping
// rules and a date-skew window, and a hand-rolled one is a bucket read waiting to
// happen. The SDK also brings the credential chain (environment, shared config,
// IRSA/IMDS, web identity), which is the part a deployment gets right for free
// and gets subtly wrong by hand.
//
// What this file deliberately does NOT do is invent an object-storage service for
// the local stack. There is no MinIO, no LocalStack and no fake bucket in
// docker-compose.yml, because a test double that behaves like S3 is a claim
// nobody can check. Instead the adapter is written against the small s3API
// interface below, and the contract suite runs against a test double - so the
// adapter's own logic (key rules, size, content type, expiry clamping, error
// mapping) is genuinely tested, while the wire behaviour stays the SDK's problem.
type S3Store struct {
	bucket string
	api    s3API
}

// s3API is the four calls this adapter makes.
//
// It exists so the adapter can be tested without a bucket, credentials or a
// network, and so that the SDK client is the only thing that knows about SDK
// types. s3APIClient below is the production implementation; s3Stub in the tests
// is the other one.
type s3API interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	PresignGetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

// S3Config is everything NewS3 needs. Endpoint, AccessKeyID and
// SecretAccessKey are optional: an empty Endpoint means AWS, and empty
// credentials mean "use the SDK's chain", which is what a deployment on IAM roles
// wants. Bucket and Region are required, because a store with no bucket has
// nowhere to put a byte and a store with no region cannot sign.
type S3Config struct {
	Bucket   string
	Region   string
	Endpoint string
	// AccessKeyID and SecretAccessKey are static credentials. They exist for
	// S3-compatible endpoints that need them; on AWS, leave them empty.
	AccessKeyID     string
	SecretAccessKey string
	// UsePathStyle is required by most S3-compatible servers and is wrong for
	// AWS. It is a setting rather than a guess.
	UsePathStyle bool
}

// NewS3 builds the production adapter from the SDK, failing closed.
//
// Every failure here is ErrNotConfigured, carrying what is missing. The point is
// that a deployment which selects the S3 driver and forgets a bucket gets a
// startup failure naming the bucket, not a service that starts, accepts an
// upload, and quietly writes it to a local directory.
func NewS3(ctx context.Context, config S3Config) (*S3Store, error) {
	bucket := strings.TrimSpace(config.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("%w: the s3 driver needs S3_BUCKET", ErrNotConfigured)
	}
	region := strings.TrimSpace(config.Region)
	if region == "" {
		return nil, fmt.Errorf("%w: the s3 driver needs S3_REGION", ErrNotConfigured)
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if endpoint := strings.TrimSpace(config.Endpoint); endpoint != "" {
		loadOptions = append(loadOptions, awsconfig.WithBaseEndpoint(endpoint))
	}
	accessKey := strings.TrimSpace(config.AccessKeyID)
	secretKey := strings.TrimSpace(config.SecretAccessKey)
	if accessKey != "" || secretKey != "" {
		if accessKey == "" || secretKey == "" {
			// Half a credential pair is never a configuration anybody meant.
			return nil, fmt.Errorf("%w: the s3 driver needs both S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY, or neither", ErrNotConfigured)
		}
		provider := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(provider))
	}

	awsConfiguration, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("%w: the aws configuration could not be loaded: %v", ErrNotConfigured, err)
	}
	client := s3.NewFromConfig(awsConfiguration, func(options *s3.Options) {
		options.UsePathStyle = config.UsePathStyle
	})
	return newS3WithAPI(bucket, s3APIClient{client: client, presign: s3.NewPresignClient(client)}), nil
}

// newS3WithAPI is the seam the contract suite uses. It is unexported on purpose:
// a caller that wants an S3 store with a caller-supplied client is a test or a
// wrapper, and both are better served by this than by making the production
// constructor take an interface that a deployment could fill with something that
// is not S3.
func newS3WithAPI(bucket string, api s3API) *S3Store {
	return &S3Store{bucket: strings.TrimSpace(bucket), api: api}
}

// Bucket reports the bucket this store writes to. It is configuration, not
// content, and it exists so a startup log can name it.
func (s *S3Store) Bucket() string {
	if s == nil {
		return ""
	}
	return s.bucket
}

func (s *S3Store) Put(ctx context.Context, key string, body io.Reader, contentType string) (Object, error) {
	if err := s.ready(key); err != nil {
		return Object{}, err
	}
	// The caller compares the reported size against the bytes it handed over and
	// deletes the object if they disagree, so this adapter has to report a size
	// rather than zero. A seekable body knows its own length; anything else is
	// counted as it is read. (PutObjectOutput has no length, so "ask the SDK"
	// is not on the table.)
	counter := &countingReader{source: body}
	if seeker, ok := body.(io.Seeker); ok {
		if position, err := seeker.Seek(0, io.SeekCurrent); err == nil {
			counter.declared = position
		}
	}
	output, err := s.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        counter,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return Object{}, fmt.Errorf("storage: put %s: %w", key, err)
	}
	// PutObject echoes back no content type, so the label the upload boundary
	// verified is the label this reports. S3 stores the object's own metadata
	// from the request, so a later Get agrees.
	_ = output
	return Object{Key: key, ContentType: contentType, Size: counter.read()}, nil
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	if err := s.ready(key); err != nil {
		return nil, Object{}, err
	}
	output, err := s.api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, Object{}, mapS3Error(key, err)
	}
	if output == nil || output.Body == nil {
		return nil, Object{}, ErrNotFound
	}
	object := Object{Key: key, ContentType: aws.ToString(output.ContentType)}
	if output.ContentLength != nil {
		object.Size = *output.ContentLength
	}
	return output.Body, object, nil
}

func (s *S3Store) SignedURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	if err := s.ready(key); err != nil {
		return "", err
	}
	// NormalizeExpiry is the SHARED policy: one ceiling, one default, both
	// adapters. The SDK can sign for longer - up to a day - and this deliberately
	// asks it for less, because a policy that is a property of the code rather
	// than a property of the cloud provider is a policy a reviewer can read.
	expiry, err := NormalizeExpiry(expires)
	if err != nil {
		return "", err
	}
	presigned, err := s.api.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = expiry
	})
	if err != nil {
		return "", fmt.Errorf("storage: presign %s: %w", key, err)
	}
	if presigned == nil || presigned.URL == "" {
		return "", fmt.Errorf("%w: the s3 client produced no presigned url for %s", ErrNotConfigured, key)
	}
	return presigned.URL, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	if err := s.ready(key); err != nil {
		return err
	}
	if _, err := s.api.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return mapS3Error(key, err)
	}
	return nil
}

func (s *S3Store) ready(key string) error {
	if s == nil || s.api == nil {
		return fmt.Errorf("%w: the s3 store has no client", ErrNotConfigured)
	}
	if s.bucket == "" {
		return fmt.Errorf("%w: the s3 store has no bucket", ErrNotConfigured)
	}
	if !ValidKey(key) {
		return ErrInvalidKey
	}
	return nil
}

// mapS3Error turns the SDK's error vocabulary into this package's, so a caller
// that asks for a missing object gets ErrNotFound whether the store is a
// directory or a bucket. Without this, every `errors.Is(err, ErrNotFound)` in the
// application would be true for the local adapter and false in production.
func mapS3Error(key string, err error) error {
	var missing *types.NotFound
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &missing) || errors.As(err, &noSuchKey) {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return fmt.Errorf("storage: %s: %w", key, err)
}

// s3APIClient is the production s3API. It exists only to bind the SDK's two
// clients - the plain one and the presigner - behind the one interface, because
// *s3.Client has no PresignGetObject and *s3.PresignClient has no PutObject.
type s3APIClient struct {
	client  *s3.Client
	presign *s3.PresignClient
}

func (c s3APIClient) PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return c.client.PutObject(ctx, in, optFns...)
}

func (c s3APIClient) GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return c.client.GetObject(ctx, in, optFns...)
}

func (c s3APIClient) DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return c.client.DeleteObject(ctx, in, optFns...)
}

func (c s3APIClient) PresignGetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	return c.presign.PresignGetObject(ctx, in, optFns...)
}

// compile-time proof that the production wiring satisfies the interface the tests
// substitute for. If a future SDK release changes a signature, this fails to
// compile here rather than at the first upload.
var _ s3API = s3APIClient{}
var _ Store = (*S3Store)(nil)
var _ Store = (*LocalStore)(nil)

// countingReader reports how many bytes were actually consumed, and remembers the
// length a seekable body declared before it started. The declared length wins
// because a body the SDK reads through more than once (a retry) would otherwise
// be reported as more bytes than were uploaded.
type countingReader struct {
	source   io.Reader
	count    int64
	declared int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	read, err := r.source.Read(p)
	r.count += int64(read)
	return read, err
}

func (r *countingReader) read() int64 {
	if r.declared > 0 {
		return r.declared
	}
	return r.count
}
