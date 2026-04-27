//go:build integration

package oras

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// pushTarGzArtifact は memory store に tar.gz artifact を push し、
// (manifest digest, layer mediaType) を返す。
func pushTarGzArtifact(t *testing.T, store *memory.Store, tag string, files map[string][]byte) string {
	t.Helper()
	ctx := context.Background()

	// tar.gz blob を構築
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("write tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	tarBytes := buf.Bytes()

	const layerMediaType = "application/vnd.tazuna.oras.layer.v1+tar+gzip"
	layerDesc := ocispec.Descriptor{
		MediaType: layerMediaType,
		Digest:    digest.FromBytes(tarBytes),
		Size:      int64(len(tarBytes)),
	}
	if err := store.Push(ctx, layerDesc, bytes.NewReader(tarBytes)); err != nil {
		t.Fatalf("push layer: %v", err)
	}

	manifestDesc, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1,
		"application/vnd.tazuna.oras.config.v1+json",
		oras.PackManifestOptions{Layers: []ocispec.Descriptor{layerDesc}},
	)
	if err != nil {
		t.Fatalf("pack manifest: %v", err)
	}
	if err := store.Tag(ctx, manifestDesc, tag); err != nil {
		t.Fatalf("tag manifest: %v", err)
	}
	// digest reference でも resolve できるようにしておく。
	if err := store.Tag(ctx, manifestDesc, manifestDesc.Digest.String()); err != nil {
		t.Fatalf("tag by digest: %v", err)
	}
	return manifestDesc.Digest.String()
}

// --- テスト本体 -------------------------------------------------------------

const testRegistry = "registry.test/example"

func newTestPuller(t *testing.T, store *memory.Store) (*spyFactory, Puller) {
	t.Helper()
	spy := &spyFactory{store: store}
	p := NewCachingPullerWithLimits(spy.factory(), DefaultLimits())
	return spy, p
}

type spyFactory struct {
	store *memory.Store
	calls int32
	err   error
}

func (s *spyFactory) factory() RepositoryFactory {
	return func(ctx context.Context, spec v1.ManifestORAS) (oras.ReadOnlyTarget, error) {
		atomic.AddInt32(&s.calls, 1)
		if s.err != nil {
			return nil, s.err
		}
		return s.store, nil
	}
}

func (s *spyFactory) callCount() int { return int(atomic.LoadInt32(&s.calls)) }

func newFixtureFiles() map[string][]byte {
	return map[string][]byte{
		"kustomization.yaml": readFixture("simple-kustomize/kustomization.yaml"),
		"deployment.yaml":    readFixture("simple-kustomize/deployment.yaml"),
	}
}

func readFixture(rel string) []byte {
	b, err := os.ReadFile(filepath.Join("testdata", "fixtures", rel))
	if err != nil {
		panic(err)
	}
	return b
}

func TestCachingPuller_TagReference(t *testing.T) {
	store := memory.New()
	files := newFixtureFiles()
	pushTarGzArtifact(t, store, "v1", files)

	spy, p := newTestPuller(t, store)
	cacheDir := t.TempDir()

	res, err := p.Pull(context.Background(), nil,
		v1.ManifestORAS{Reference: testRegistry + ":v1"},
		PullOptions{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if res.LocalPath == "" {
		t.Fatal("LocalPath is empty")
	}
	if !strings.HasPrefix(res.Digest, "sha256:") {
		t.Fatalf("unexpected digest %q", res.Digest)
	}
	if got := mustReadFile(t, filepath.Join(res.LocalPath, "kustomization.yaml")); !bytes.Equal(got, files["kustomization.yaml"]) {
		t.Errorf("kustomization.yaml content mismatch")
	}
	if got := mustReadFile(t, filepath.Join(res.LocalPath, "deployment.yaml")); !bytes.Equal(got, files["deployment.yaml"]) {
		t.Errorf("deployment.yaml content mismatch")
	}
	if spy.callCount() != 1 {
		t.Errorf("factory called %d times, want 1", spy.callCount())
	}
}

func TestCachingPuller_DigestReference(t *testing.T) {
	store := memory.New()
	files := newFixtureFiles()
	dig := pushTarGzArtifact(t, store, "v1", files)

	spy, p := newTestPuller(t, store)
	cacheDir := t.TempDir()

	res, err := p.Pull(context.Background(), nil,
		v1.ManifestORAS{Reference: testRegistry + "@" + dig},
		PullOptions{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if res.Digest != dig {
		t.Errorf("digest = %q, want %q", res.Digest, dig)
	}
	if spy.callCount() != 1 {
		t.Errorf("factory called %d times, want 1", spy.callCount())
	}
}

func TestCachingPuller_CacheHit(t *testing.T) {
	store := memory.New()
	pushTarGzArtifact(t, store, "v1", newFixtureFiles())

	spy, p := newTestPuller(t, store)
	cacheDir := t.TempDir()
	spec := v1.ManifestORAS{Reference: testRegistry + ":v1"}

	if _, err := p.Pull(context.Background(), nil, spec, PullOptions{CacheDir: cacheDir}); err != nil {
		t.Fatalf("first pull: %v", err)
	}
	if spy.callCount() != 1 {
		t.Fatalf("after first pull factory calls = %d, want 1", spy.callCount())
	}

	// 2回目: tag は既知だが事前マッピングがあるので factory 非呼び出しで cache hit する。
	if _, err := p.Pull(context.Background(), nil, spec, PullOptions{CacheDir: cacheDir}); err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if spy.callCount() != 1 {
		t.Errorf("after second pull factory calls = %d, want 1 (cache hit expected)", spy.callCount())
	}
}

func TestCachingPuller_NoCacheBypassesCache(t *testing.T) {
	store := memory.New()
	pushTarGzArtifact(t, store, "v1", newFixtureFiles())

	spy, p := newTestPuller(t, store)
	cacheDir := t.TempDir()
	spec := v1.ManifestORAS{Reference: testRegistry + ":v1"}

	if _, err := p.Pull(context.Background(), nil, spec, PullOptions{CacheDir: cacheDir}); err != nil {
		t.Fatalf("first pull: %v", err)
	}
	if _, err := p.Pull(context.Background(), nil, spec, PullOptions{CacheDir: cacheDir, NoCache: true}); err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if spy.callCount() != 2 {
		t.Errorf("factory calls = %d, want 2", spy.callCount())
	}
}

func TestCachingPuller_OfflineMissError(t *testing.T) {
	store := memory.New()
	pushTarGzArtifact(t, store, "v1", newFixtureFiles())

	spy, p := newTestPuller(t, store)
	cacheDir := t.TempDir()

	_, err := p.Pull(context.Background(), nil,
		v1.ManifestORAS{Reference: testRegistry + ":v1"},
		PullOptions{CacheDir: cacheDir, Offline: true})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "offline") {
		t.Errorf("error %q does not mention offline", err.Error())
	}
	if spy.callCount() != 0 {
		t.Errorf("factory called %d times in offline+miss path, want 0", spy.callCount())
	}
}

func TestCachingPuller_OfflineHitWithTag(t *testing.T) {
	store := memory.New()
	pushTarGzArtifact(t, store, "v1", newFixtureFiles())

	spy, p := newTestPuller(t, store)
	cacheDir := t.TempDir()
	spec := v1.ManifestORAS{Reference: testRegistry + ":v1"}

	// 事前に1回 online で pull してキャッシュと ref マッピングを作る。
	if _, err := p.Pull(context.Background(), nil, spec, PullOptions{CacheDir: cacheDir}); err != nil {
		t.Fatalf("warmup pull: %v", err)
	}
	warmupCalls := spy.callCount()

	res, err := p.Pull(context.Background(), nil, spec, PullOptions{CacheDir: cacheDir, Offline: true})
	if err != nil {
		t.Fatalf("offline pull after warmup: %v", err)
	}
	if !strings.HasPrefix(res.Digest, "sha256:") {
		t.Errorf("unexpected digest %q", res.Digest)
	}
	if spy.callCount() != warmupCalls {
		t.Errorf("factory called %d times during offline hit, want %d (no additional calls)",
			spy.callCount(), warmupCalls)
	}
}

func TestCachingPuller_ExtractorErrorPropagates(t *testing.T) {
	store := memory.New()
	// `..` を含む不正な entry を含むtar.gzを push する。
	pushTarGzArtifact(t, store, "v1", map[string][]byte{
		"../evil.txt": []byte("escaped!"),
	})

	_, p := newTestPuller(t, store)
	cacheDir := t.TempDir()

	_, err := p.Pull(context.Background(), nil,
		v1.ManifestORAS{Reference: testRegistry + ":v1"},
		PullOptions{CacheDir: cacheDir})
	if err == nil {
		t.Fatal("expected extractor error, got nil")
	}
	if !strings.Contains(err.Error(), "extract") {
		t.Errorf("error %q does not mention extract", err.Error())
	}
}

func TestCachingPuller_ResolveErrorPropagates(t *testing.T) {
	store := memory.New() // 空 store
	_, p := newTestPuller(t, store)
	cacheDir := t.TempDir()

	_, err := p.Pull(context.Background(), nil,
		v1.ManifestORAS{Reference: testRegistry + ":nonexistent"},
		PullOptions{CacheDir: cacheDir})
	if err == nil {
		t.Fatal("expected resolve error, got nil")
	}
	if !strings.Contains(err.Error(), "resolve") {
		t.Errorf("error %q does not mention resolve", err.Error())
	}
}

func TestCachingPuller_FactoryError(t *testing.T) {
	store := memory.New()
	pushTarGzArtifact(t, store, "v1", newFixtureFiles())

	spy := &spyFactory{store: store, err: errors.New("boom")}
	p := NewCachingPuller(spy.factory())
	cacheDir := t.TempDir()

	_, err := p.Pull(context.Background(), nil,
		v1.ManifestORAS{Reference: testRegistry + ":v1"},
		PullOptions{CacheDir: cacheDir})
	if err == nil {
		t.Fatal("expected factory error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not contain factory message", err.Error())
	}
}

func TestResolveCacheDir(t *testing.T) {
	tests := []struct {
		name     string
		override string
		env      map[string]string
		want     string // contains substring
	}{
		{
			name:     "explicit override",
			override: t.TempDir(),
			want:     "", // sentinel: equal to override
		},
		{
			name: "XDG_CACHE_HOME",
			env:  map[string]string{"XDG_CACHE_HOME": "/tmp/xdg-cache"},
			want: filepath.Join("/tmp/xdg-cache", "tazuna", "oras"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := resolveCacheDir(tt.override)
			if err != nil {
				t.Fatalf("resolveCacheDir: %v", err)
			}
			if tt.override != "" {
				if got != tt.override {
					t.Errorf("got %q, want %q", got, tt.override)
				}
				return
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- 共通ヘルパー -------------------------------------------------------------

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
