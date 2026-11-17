package tfstate

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

// S3Backend は S3 (および S3 互換ストレージ) 上の tfstate を読む Backend です。
// terraform の `backend "s3"` と同じ bucket / key / workspace の規則でオブジェクトを
// 特定します。認証情報は AWS SDK 標準の credential chain で解決します。
type S3Backend struct {
	bucket       string
	key          string
	region       string
	profile      string
	endpoint     string
	usePathStyle bool
}

var _ Backend = (*S3Backend)(nil)

// NewS3Backend は BackendFactory です。AWS の設定や認証情報のロードは Read まで遅延します。
func NewS3Backend(cfg v1.TFStateBackend, _ string) (Backend, error) {
	c := cfg.S3
	if c == nil {
		return nil, errors.New("s3 config is required for s3 backend")
	}
	if c.Bucket == "" {
		return nil, errors.New("s3.bucket is required")
	}
	if c.Key == "" {
		return nil, errors.New("s3.key is required")
	}
	return &S3Backend{
		bucket:       c.Bucket,
		key:          S3ObjectKey(c),
		region:       c.Region,
		profile:      c.Profile,
		endpoint:     c.Endpoint,
		usePathStyle: c.UsePathStyle,
	}, nil
}

// S3ObjectKey は workspace を考慮した実際のオブジェクトキーを返します。
// terraform の s3 backend と同じく、workspace が空または "default" なら key を
// そのまま使い、それ以外は `<workspaceKeyPrefix>/<workspace>/<key>` になります。
func S3ObjectKey(c *v1.TFStateS3Backend) string {
	if c.Workspace == "" || c.Workspace == "default" {
		return c.Key
	}
	prefix := c.WorkspaceKeyPrefix
	if prefix == "" {
		prefix = v1.DefaultS3WorkspaceKeyPrefix
	}
	return strings.TrimSuffix(prefix, "/") + "/" + c.Workspace + "/" + c.Key
}

// Read implements Backend.
func (b *S3Backend) Read(ctx context.Context) ([]byte, error) {
	client, err := b.newClient(ctx)
	if err != nil {
		return nil, err
	}

	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key),
	})
	if err != nil {
		return nil, b.wrapGetObjectError(err)
	}
	data, readErr := io.ReadAll(out.Body)
	closeErr := out.Body.Close()
	if readErr != nil {
		return nil, errors.Wrapf(readErr, "failed to read tfstate object %s", b.location())
	}
	if closeErr != nil {
		return nil, errors.Wrapf(closeErr, "failed to close tfstate object %s", b.location())
	}
	return data, nil
}

func (b *S3Backend) newClient(ctx context.Context) (*s3.Client, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if b.region != "" {
		opts = append(opts, awsconfig.WithRegion(b.region))
	}
	if b.profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(b.profile))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load AWS config (check AWS_PROFILE / s3.profile and credentials)")
	}
	if cfg.Region == "" {
		return nil, errors.Errorf("AWS region is not configured for %s; set s3.region or AWS_REGION", b.location())
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if b.endpoint != "" {
			o.BaseEndpoint = aws.String(b.endpoint)
		}
		o.UsePathStyle = b.usePathStyle
	}), nil
}

func (b *S3Backend) wrapGetObjectError(err error) error {
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return errors.Wrapf(err, "tfstate object %s not found (check s3.bucket / s3.key / s3.workspace)", b.location())
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchBucket":
			return errors.Wrapf(err, "S3 bucket %q not found", b.bucket)
		case "AccessDenied", "Forbidden":
			return errors.Wrapf(err, "access denied to tfstate object %s (s3:GetObject, and kms:Decrypt for SSE-KMS buckets, is required)", b.location())
		}
	}
	return errors.Wrapf(err, "failed to get tfstate object %s", b.location())
}

func (b *S3Backend) location() string {
	return "s3://" + b.bucket + "/" + b.key
}

// LogAttrs implements Backend.
func (b *S3Backend) LogAttrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("bucket", b.bucket),
		slog.String("key", b.key),
	}
	if b.region != "" {
		attrs = append(attrs, slog.String("region", b.region))
	}
	if b.profile != "" {
		attrs = append(attrs, slog.String("profile", b.profile))
	}
	if b.endpoint != "" {
		attrs = append(attrs, slog.String("endpoint", b.endpoint))
	}
	return attrs
}
