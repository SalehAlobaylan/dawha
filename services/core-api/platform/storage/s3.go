package storage

import (
	"bytes"
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
// What this file deliberately does NOT do is put a fake bucket in the default
// local stack. MinIO is in docker-compose.yml, but behind the `storage` profile
// and started only by `make storage-up`, because storage is not something the
// default developer loop should pay for. The adapter is written against the small
// s3API interface below, so the contract suite runs against a test double with no
// bucket and no network - and, separately and opt-in, against a real MinIO with
// the same assertions (minio_test.go).
//
// The split is not tidiness. The double is fast and unconditional, and it is where
// the adapter's own logic is tested. The MinIO case is where the wire is tested,
// and it is the only one of the two that can catch a disagreement between this
// adapter and S3: it already caught two, a Put that no plaintext S3 endpoint
// accepts and a presign whose determinism no real signer has. A double that
// behaves like S3 is a claim; the claim was wrong twice, and the double is the
// reason nobody noticed.
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
	// rather than zero. PutObjectOutput has no length, so "ask the SDK" is not on
	// the table: the length has to be settled before the call, which is what
	// newPutBody does.
	payload, err := newPutBody(body)
	if err != nil {
		return Object{}, fmt.Errorf("storage: put %s: %w", key, err)
	}
	output, err := s.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        payload,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return Object{}, fmt.Errorf("storage: put %s: %w", key, err)
	}
	// PutObject echoes back no content type, so the label the upload boundary
	// verified is the label this reports. S3 stores the object's own metadata
	// from the request, so a later Get agrees.
	_ = output
	return Object{Key: key, ContentType: contentType, Size: payload.length()}, nil
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

// putBody is the body Put hands to the SDK, and the number of bytes that body
// will send.
//
// The length is known before the call rather than counted during it, and the
// reason is the SDK. The AWS SDK computes a request checksum for PutObject by
// reading the body and then REWINDING it (service/internal/checksum's
// compute-input-checksum middleware), and when the endpoint is plain HTTP it has
// no trailing-checksum alternative to fall back on: over TLS it can send a
// trailing checksum while the stream goes past, over HTTP it cannot, so the body
// has to be seekable. An io.Pipe here therefore fails with "unseekable stream is
// not supported without TLS and trailing checksum" - and the same middleware
// re-reads a seekable body, so a count taken across the call is not a number the
// adapter can report with a straight face either.
//
// So every body this adapter uploads is rewound, and the length is the length of
// the part that gets sent. That is a stronger statement than a byte count: it is
// the number the upload boundary is comparing against, computed from the same
// stream the server will read.
type putBody interface {
	io.Reader
	// length is how many bytes the SDK will read from this body.
	length() int64
}

// newPutBody settles the length and returns something the SDK can rewind.
//
// A seekable body is wrapped, not copied: Read and Seek are delegated, so a file
// or a bytes.Reader is streamed and the caller pays for no extra copy. A body
// that cannot seek is read into memory, because the alternative is an upload
// that fails against every plaintext S3 endpoint for a reason that has nothing
// to do with the object. That is a documented cost rather than a free one: the
// buffered case holds the object in memory once. In this repository the only
// production caller already has the whole object in memory before it calls
// (upload.go hands over a *bytes.Reader), so the buffer is a copy of something
// the caller was holding anyway - and the local adapter, which streams to a
// temporary file, reports the same size for the same body, which is what the
// shared contract asserts.
func newPutBody(body io.Reader) (putBody, error) {
	if body == nil {
		return nil, errors.New("the body is nil")
	}
	seeker, ok := body.(io.ReadSeeker)
	if !ok {
		buffered, err := io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("the body could not be buffered: %w", err)
		}
		return &bufferedPutBody{reader: bytes.NewReader(buffered), remaining: int64(len(buffered))}, nil
	}
	// The length of the REMAINDER, not of the whole thing and not the offset. The
	// SDK records the position the body was handed over at and uploads from
	// there (smithy-go's streamLength does this deliberately, so an application
	// that has already read a header off a file uploads the rest), and the size
	// this adapter reports has to be the size of the same part.
	start, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("the body's position could not be read: %w", err)
	}
	end, err := seeker.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("the body's length could not be read: %w", err)
	}
	if _, err := seeker.Seek(start, io.SeekStart); err != nil {
		return nil, fmt.Errorf("the body could not be rewound: %w", err)
	}
	if end < start {
		return nil, fmt.Errorf("the body reported a length of %d from a position of %d", end, start)
	}
	return &seekablePutBody{seeker: seeker, remaining: end - start}, nil
}

// seekablePutBody forwards to a body that could already seek. It exists only so
// the SDK sees an io.Seeker, which it type-checks before it will rewind.
type seekablePutBody struct {
	seeker    io.ReadSeeker
	remaining int64
}

func (b *seekablePutBody) Read(p []byte) (int, error) { return b.seeker.Read(p) }

func (b *seekablePutBody) Seek(offset int64, whence int) (int64, error) {
	return b.seeker.Seek(offset, whence)
}

func (b *seekablePutBody) length() int64 { return b.remaining }

// bufferedPutBody is the non-seekable case, already read into memory. bytes.Reader
// is seekable, so the SDK can rewind it, and the length is remembered here rather
// than asked of the reader afterwards: by the time the call returns, the reader
// has been drained and would report zero.
type bufferedPutBody struct {
	reader    *bytes.Reader
	remaining int64
}

func (b *bufferedPutBody) Read(p []byte) (int, error) { return b.reader.Read(p) }

func (b *bufferedPutBody) length() int64 { return b.remaining }
