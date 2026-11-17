package tfstate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/tfstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactories_New(t *testing.T) {
	t.Parallel()
	f := tfstate.DefaultFactories()

	tests := []struct {
		name    string
		cfg     v1.TFStateBackend
		wantErr string
	}{
		{"missing type", v1.TFStateBackend{}, "backend.type is required"},
		{"unknown type", v1.TFStateBackend{Type: "gcs"}, `unsupported tfstate backend type "gcs" (supported: local, s3)`},
		{"s3 without config", v1.TFStateBackend{Type: "s3"}, "s3 config is required"},
		{"s3 without bucket", v1.TFStateBackend{Type: "s3", S3: &v1.TFStateS3Backend{Key: "k"}}, "s3.bucket is required"},
		{"s3 without key", v1.TFStateBackend{Type: "s3", S3: &v1.TFStateS3Backend{Bucket: "b"}}, "s3.key is required"},
		{"local without config", v1.TFStateBackend{Type: "local"}, "local config is required"},
		{"local without path", v1.TFStateBackend{Type: "local", Local: &v1.TFStateLocalBackend{}}, "local.path is required"},
		{"s3 ok", v1.TFStateBackend{Type: "s3", S3: &v1.TFStateS3Backend{Bucket: "b", Key: "k"}}, ""},
		{"local ok", v1.TFStateBackend{Type: "local", Local: &v1.TFStateLocalBackend{Path: "x"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := f.New(tt.cfg, "")
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.NotNil(t, b)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLocalBackend(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terraform.tfstate"), []byte(`{"version":4}`), 0o600))

	// 相対 path は baseDir から解決される
	b, err := tfstate.NewLocalBackend(v1.TFStateBackend{Type: "local", Local: &v1.TFStateLocalBackend{Path: "terraform.tfstate"}}, dir)
	require.NoError(t, err)
	data, err := b.Read(context.Background())
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":4}`, string(data))
	assert.Equal(t, filepath.Join(dir, "terraform.tfstate"), b.LogAttrs()[0].Value.String())

	missing, err := tfstate.NewLocalBackend(v1.TFStateBackend{Type: "local", Local: &v1.TFStateLocalBackend{Path: filepath.Join(dir, "nope.tfstate")}}, "/ignored")
	require.NoError(t, err)
	_, err = missing.Read(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read tfstate file")
}

func TestS3ObjectKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  v1.TFStateS3Backend
		want string
	}{
		{"no workspace", v1.TFStateS3Backend{Key: "net/terraform.tfstate"}, "net/terraform.tfstate"},
		{"default workspace", v1.TFStateS3Backend{Key: "net/terraform.tfstate", Workspace: "default"}, "net/terraform.tfstate"},
		{"workspace with default prefix", v1.TFStateS3Backend{Key: "net/terraform.tfstate", Workspace: "prod"}, "env:/prod/net/terraform.tfstate"},
		{"workspace with custom prefix", v1.TFStateS3Backend{Key: "net.tfstate", Workspace: "prod", WorkspaceKeyPrefix: "workspaces"}, "workspaces/prod/net.tfstate"},
		{"prefix with trailing slash", v1.TFStateS3Backend{Key: "net.tfstate", Workspace: "prod", WorkspaceKeyPrefix: "ws/"}, "ws/prod/net.tfstate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tfstate.S3ObjectKey(&tt.cfg))
		})
	}
}

// fakeS3 は path-style の GetObject だけを返す最小限の S3 互換サーバです。
type fakeS3 struct {
	t        *testing.T
	mu       sync.Mutex
	objects  map[string]string // "/<bucket>/<key>" -> body
	denied   map[string]bool   // "/<bucket>/<key>" -> AccessDenied
	requests []*http.Request
}

func (f *fakeS3) write(w http.ResponseWriter, body string) {
	if _, err := w.Write([]byte(body)); err != nil {
		f.t.Errorf("fake S3: failed to write response: %v", err)
	}
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Clone(context.Background()))
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/xml")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Path
	if f.denied[path] {
		w.WriteHeader(http.StatusForbidden)
		f.write(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
		return
	}
	if strings.HasPrefix(path, "/missing-bucket/") {
		w.WriteHeader(http.StatusNotFound)
		f.write(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist</Message></Error>`)
		return
	}
	body, ok := f.objects[path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		f.write(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	f.write(w, body)
}

// isolateAWSEnv は開発者の AWS 設定 (AWS_PROFILE 等) にテストが影響されないようにし、
// 固定のダミー認証情報を使わせます。t.Setenv を使うため呼び出し側は t.Parallel 不可です。
func isolateAWSEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	t.Setenv("AWS_CONFIG_FILE", empty)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", empty)
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDTESTTAZUNA")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy-secret")
}

func newS3Backend(t *testing.T, endpoint string, c v1.TFStateS3Backend) tfstate.Backend {
	t.Helper()
	c.Endpoint = endpoint
	c.UsePathStyle = true
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	b, err := tfstate.NewS3Backend(v1.TFStateBackend{Type: "s3", S3: &c}, "")
	require.NoError(t, err)
	return b
}

func TestS3Backend_Read(t *testing.T) {
	isolateAWSEnv(t)

	fake := &fakeS3{
		t: t,
		objects: map[string]string{
			"/tfstate-bucket/network/terraform.tfstate":           `{"version":4,"serial":1}`,
			"/tfstate-bucket/env:/prod/network/terraform.tfstate": `{"version":4,"serial":2}`,
		},
		denied: map[string]bool{"/tfstate-bucket/secret.tfstate": true},
	}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ctx := context.Background()

	t.Run("default workspace", func(t *testing.T) {
		b := newS3Backend(t, srv.URL, v1.TFStateS3Backend{Bucket: "tfstate-bucket", Key: "network/terraform.tfstate"})
		data, err := b.Read(ctx)
		require.NoError(t, err)
		assert.JSONEq(t, `{"version":4,"serial":1}`, string(data))

		fake.mu.Lock()
		last := fake.requests[len(fake.requests)-1]
		fake.mu.Unlock()
		// SigV4 で署名されていること (credential chain から認証情報を得ている)
		assert.Contains(t, last.Header.Get("Authorization"), "Credential=AKIDTESTTAZUNA/")
		assert.Contains(t, last.Header.Get("Authorization"), "/us-east-1/s3/aws4_request")
	})

	t.Run("workspace", func(t *testing.T) {
		b := newS3Backend(t, srv.URL, v1.TFStateS3Backend{Bucket: "tfstate-bucket", Key: "network/terraform.tfstate", Workspace: "prod"})
		data, err := b.Read(ctx)
		require.NoError(t, err)
		assert.JSONEq(t, `{"version":4,"serial":2}`, string(data))
	})

	t.Run("no such key", func(t *testing.T) {
		b := newS3Backend(t, srv.URL, v1.TFStateS3Backend{Bucket: "tfstate-bucket", Key: "network/terraform.tfstate", Workspace: "staging"})
		_, err := b.Read(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tfstate object s3://tfstate-bucket/env:/staging/network/terraform.tfstate not found")
	})

	t.Run("access denied", func(t *testing.T) {
		b := newS3Backend(t, srv.URL, v1.TFStateS3Backend{Bucket: "tfstate-bucket", Key: "secret.tfstate"})
		_, err := b.Read(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "access denied to tfstate object s3://tfstate-bucket/secret.tfstate")
	})

	t.Run("no such bucket", func(t *testing.T) {
		b := newS3Backend(t, srv.URL, v1.TFStateS3Backend{Bucket: "missing-bucket", Key: "x.tfstate"})
		_, err := b.Read(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `S3 bucket "missing-bucket" not found`)
	})

	t.Run("log attrs do not contain credentials", func(t *testing.T) {
		b := newS3Backend(t, srv.URL, v1.TFStateS3Backend{Bucket: "tfstate-bucket", Key: "k", Profile: "dev"})
		var keys []string
		for _, a := range b.LogAttrs() {
			keys = append(keys, a.Key)
			assert.NotContains(t, a.Value.String(), "dummy-secret")
		}
		assert.Equal(t, []string{"bucket", "key", "region", "profile", "endpoint"}, keys)
	})
}

func TestS3Backend_MissingRegion(t *testing.T) {
	isolateAWSEnv(t)

	b, err := tfstate.NewS3Backend(v1.TFStateBackend{Type: "s3", S3: &v1.TFStateS3Backend{Bucket: "b", Key: "k"}}, "")
	require.NoError(t, err)
	_, err = b.Read(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS region is not configured for s3://b/k")
}

func TestS3Backend_UnknownProfile(t *testing.T) {
	isolateAWSEnv(t)

	b, err := tfstate.NewS3Backend(v1.TFStateBackend{Type: "s3", S3: &v1.TFStateS3Backend{Bucket: "b", Key: "k", Region: "us-east-1", Profile: "does-not-exist"}}, "")
	require.NoError(t, err)
	_, err = b.Read(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load AWS config")
}
