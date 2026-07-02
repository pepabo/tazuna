// Package oras - puller.go
//
// registry から OCI artifact を pull し、tar.gz layer を
// extractor で展開してローカルキャッシュに格納する。
// digest ベースでキャッシュを共有するため、同一 digest の 2 回目以降の
// pull はネットワークアクセスを伴わない。
//
// 詳細は docs/adr/004-oras-manager.md および oras-manager-roadmap.md (commit 4) を参照。
package oras

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// PullOptions は Pull の挙動を制御するオプションです。
type PullOptions struct {
	// NoCache は true の場合、キャッシュを無視して必ず registry から再取得します。
	NoCache bool
	// Offline は true の場合、registry へのアクセスを禁止し、
	// キャッシュミス時はエラーを返します。
	Offline bool
	// CacheDir は明示的なキャッシュディレクトリを指定します。
	// 空の場合は $XDG_CACHE_HOME/tazuna/oras (未設定なら $HOME/.cache/tazuna/oras) を使用します。
	CacheDir string
}

// PullResult は Pull の成功時の戻り値です。
type PullResult struct {
	// LocalPath は展開先ディレクトリの絶対パスです。
	// 呼び出し側はこのパス配下に対して spec.Target でサブパスを解決します。
	LocalPath string
	// Digest は解決済みの manifest digest (例: "sha256:abc...") です。
	Digest string
}

// Puller は registry から artifact を pull するコンポーネントです。
type Puller interface {
	Pull(ctx context.Context, logger *slog.Logger, spec v1.ManifestORAS, opts PullOptions) (PullResult, error)
}

// RepositoryFactory は spec に対応する pull 対象 store を返します。
// production では remote.Repository を、テストでは memory.Store などを返すことで
// Puller の単体テストを net/registry 非依存にします。
type RepositoryFactory func(ctx context.Context, spec v1.ManifestORAS) (oras.ReadOnlyTarget, error)

// cachingPuller は digest ベースでキャッシュを行う Puller の実装です。
type cachingPuller struct {
	factory RepositoryFactory
	limits  Limits
}

// NewCachingPuller は cachingPuller を返します。
// limits は extractor のデフォルト (1 GiB / 10000 entries) を使用します。
func NewCachingPuller(factory RepositoryFactory) Puller {
	return &cachingPuller{factory: factory, limits: DefaultLimits()}
}

// NewCachingPullerWithLimits はテスト用に展開上限を差し替え可能にした
// cachingPuller を返します。
func NewCachingPullerWithLimits(factory RepositoryFactory, limits Limits) Puller {
	return &cachingPuller{factory: factory, limits: limits}
}

// Pull は spec.Reference をキャッシュ→ registry の順に解決し、
// 展開済みディレクトリのパスを返します。
func (p *cachingPuller) Pull(ctx context.Context, logger *slog.Logger, spec v1.ManifestORAS, opts PullOptions) (PullResult, error) {
	if logger == nil {
		logger = slog.Default()
	}

	cacheDir, err := resolveCacheDir(opts.CacheDir)
	if err != nil {
		return PullResult{}, err
	}
	blobsDir := filepath.Join(cacheDir, "blobs")
	refsDir := filepath.Join(cacheDir, "refs")

	ref, err := registry.ParseReference(spec.Reference)
	if err != nil {
		return PullResult{}, fmt.Errorf("oras pull: parse reference %q: %w", spec.Reference, err)
	}
	if ref.Reference == "" {
		return PullResult{}, fmt.Errorf("oras pull: reference %q has no tag or digest", spec.Reference)
	}

	isDigest := ref.ValidateReferenceAsDigest() == nil
	refKey := sanitizeRef(spec.Reference)

	// digest 指定の場合は即座に digest を確定させる。
	// tag 指定の場合は !NoCache なら refs マッピングから過去の解決結果を試す
	// (registry を叩かずにキャッシュヒットさせるため)。
	var resolvedDigest string
	if isDigest {
		resolvedDigest = ref.Reference
	} else if !opts.NoCache {
		if mapped, ok := readRefMapping(refsDir, refKey); ok {
			resolvedDigest = mapped
		}
	}

	// 事前キャッシュヒットチェック (registry アクセス前)。
	if !opts.NoCache && resolvedDigest != "" {
		blobDir := filepath.Join(blobsDir, sanitizeDigest(resolvedDigest))
		if dirExists(blobDir) {
			logger.Debug("oras pull cache hit",
				"ref", spec.Reference, "digest", resolvedDigest, "path", blobDir)
			return PullResult{LocalPath: blobDir, Digest: resolvedDigest}, nil
		}
	}

	if opts.Offline {
		return PullResult{}, fmt.Errorf("oras pull: offline mode and cache miss for %q", spec.Reference)
	}

	src, err := p.factory(ctx, spec)
	if err != nil {
		return PullResult{}, fmt.Errorf("oras pull: build repository for %q: %w", spec.Reference, err)
	}

	manifestDesc, err := src.Resolve(ctx, ref.Reference)
	if err != nil {
		return PullResult{}, fmt.Errorf("oras pull: resolve %q: %w", spec.Reference, err)
	}
	resolvedDigest = manifestDesc.Digest.String()
	blobDir := filepath.Join(blobsDir, sanitizeDigest(resolvedDigest))

	if opts.NoCache {
		if err := os.RemoveAll(blobDir); err != nil {
			return PullResult{}, fmt.Errorf("oras pull: clear cache dir: %w", err)
		}
	} else if dirExists(blobDir) {
		// tag → digest 解決後にキャッシュヒットしたケース。
		logger.Debug("oras pull cache hit after resolve",
			"ref", spec.Reference, "digest", resolvedDigest, "path", blobDir)
		if !isDigest {
			if err := writeRefMapping(refsDir, refKey, resolvedDigest); err != nil {
				return PullResult{}, err
			}
		}
		return PullResult{LocalPath: blobDir, Digest: resolvedDigest}, nil
	}

	manifestRC, err := src.Fetch(ctx, manifestDesc)
	if err != nil {
		return PullResult{}, fmt.Errorf("oras pull: fetch manifest: %w", err)
	}
	// リモートレジストリの場合、Close しないと HTTP response body がリークする
	manifestBytes, err := content.ReadAll(manifestRC, manifestDesc)
	closeErr := manifestRC.Close()
	if err != nil {
		return PullResult{}, fmt.Errorf("oras pull: read manifest: %w", err)
	}
	if closeErr != nil {
		return PullResult{}, fmt.Errorf("oras pull: close manifest reader: %w", closeErr)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return PullResult{}, fmt.Errorf("oras pull: parse manifest: %w", err)
	}
	if len(manifest.Layers) != 1 {
		return PullResult{}, fmt.Errorf("oras pull: expected exactly 1 layer, got %d", len(manifest.Layers))
	}

	layerRC, err := src.Fetch(ctx, manifest.Layers[0])
	if err != nil {
		return PullResult{}, fmt.Errorf("oras pull: fetch layer: %w", err)
	}
	defer func() { _ = layerRC.Close() }()

	if err := os.MkdirAll(blobsDir, 0o755); err != nil {
		return PullResult{}, fmt.Errorf("oras pull: mkdir blobs: %w", err)
	}
	tmpDir, err := os.MkdirTemp(blobsDir, "extract-*")
	if err != nil {
		return PullResult{}, fmt.Errorf("oras pull: mkdir tmp: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	if err := ExtractWithLimits(layerRC, tmpDir, p.limits); err != nil {
		cleanup()
		return PullResult{}, fmt.Errorf("oras pull: extract layer: %w", err)
	}

	if err := os.Rename(tmpDir, blobDir); err != nil {
		// 別プロセスが先に展開を完了している (or NoCache 削除後の race) ケースを許容。
		if dirExists(blobDir) {
			cleanup()
		} else {
			cleanup()
			return PullResult{}, fmt.Errorf("oras pull: finalize cache dir: %w", err)
		}
	}

	if !isDigest {
		if err := writeRefMapping(refsDir, refKey, resolvedDigest); err != nil {
			return PullResult{}, err
		}
	}

	logger.Debug("oras pull complete",
		"ref", spec.Reference, "digest", resolvedDigest, "path", blobDir)
	return PullResult{LocalPath: blobDir, Digest: resolvedDigest}, nil
}

// resolveCacheDir はキャッシュディレクトリの絶対パスを返します。
func resolveCacheDir(override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("oras pull: resolve cache dir: %w", err)
		}
		return abs, nil
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "tazuna", "oras"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("oras pull: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".cache", "tazuna", "oras"), nil
}

// refSanitizer は reference をフラットなファイル名に変換します。
var refSanitizer = strings.NewReplacer("/", "_", ":", "_", "@", "_")

func sanitizeRef(s string) string {
	return refSanitizer.Replace(s)
}

// sanitizeDigest は "sha256:abc..." 形式の digest を
// ディレクトリ名として安全な "sha256-abc..." に変換します。
func sanitizeDigest(d string) string {
	return strings.ReplaceAll(d, ":", "-")
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func writeRefMapping(refsDir, refKey, digest string) error {
	if err := os.MkdirAll(refsDir, 0o755); err != nil {
		return fmt.Errorf("oras pull: mkdir refs: %w", err)
	}
	tmp, err := os.CreateTemp(refsDir, "ref-*")
	if err != nil {
		return fmt.Errorf("oras pull: create tmp ref mapping: %w", err)
	}
	if _, err := tmp.WriteString(digest); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("oras pull: write ref mapping: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("oras pull: close ref mapping: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(refsDir, refKey)); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("oras pull: finalize ref mapping: %w", err)
	}
	return nil
}

func readRefMapping(refsDir, refKey string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(refsDir, refKey))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false
		}
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// NewRemoteRepositoryFactory は remote.Repository を生成する production 用の
// RepositoryFactory を返します。CredentialResolver を共有することで、
// 複数 manifest にまたがる auth.Cache の再利用が可能になります。
//
// resolver が nil の場合は anonymous アクセスのみ可能です。
func NewRemoteRepositoryFactory(resolver *CredentialResolver) RepositoryFactory {
	return func(ctx context.Context, spec v1.ManifestORAS) (oras.ReadOnlyTarget, error) {
		repo, err := remote.NewRepository(spec.Reference)
		if err != nil {
			return nil, fmt.Errorf("oras: create remote repository for %q: %w", spec.Reference, err)
		}
		repo.PlainHTTP = spec.PlainHTTP

		httpClient := &http.Client{}
		if spec.InsecureSkipVerify {
			httpClient.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // 設定で明示的に有効化された場合のみ
			}
		}

		client := &auth.Client{Client: httpClient}
		if resolver != nil {
			client.Cache = resolver.Cache()
			specForClosure := spec
			client.Credential = func(ctx context.Context, hostport string) (auth.Credential, error) {
				return resolver.Resolve(ctx, hostport, specForClosure.Auth)
			}
		}
		repo.Client = client
		return repo, nil
	}
}
