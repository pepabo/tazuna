package runner

import (
	"io"
	"log/slog"
	"path/filepath"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
	"github.com/pepabo/tazuna/pkg/manager"
	orasmanager "github.com/pepabo/tazuna/pkg/manager/oras"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/testplugin"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TazunaRunner はTazuna操作を管理するオーバーオールな構造体
// これを用意し、各コマンドからはこの構造体を通して操作することで、
// Tazunaのロジックをそれぞれ拡張してビルドし、利用できるようにします
type TazunaRunner struct {
	logger       *slog.Logger
	tags         []string // マニフェストに適用するタグ
	k8sClient    client.Client
	opClient     op.Client // OnePasswordのクライアント
	orasPullOpts orasmanager.PullOptions
}

type RunnerOption func(*TazunaRunner)

func NewTazunaRunner(
	logger *slog.Logger,
	k8sClient client.Client,
	opClient op.Client,
	opts ...RunnerOption) *TazunaRunner {
	// nilロガーで生成された場合のpanicを避けるため、ログを破棄するロガーへフォールバックする
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	r := TazunaRunner{
		logger:    logger,
		k8sClient: k8sClient,
		opClient:  opClient,
	}

	for _, o := range opts {
		o(&r)
	}
	return &r
}

func setupManagers(k8sClient client.Client, opClient op.Client, orasOpts orasmanager.PullOptions) map[string]manager.Manager {
	m := make(map[string]manager.Manager)
	m[string(v1.ManifestTypeGenesisSecret)] = manager.NewGenesisSecret(k8sClient, genesissecret.NewOnePasswordProvider(opClient))
	kustomizeManager := manager.NewKustomize(k8sClient)
	m[string(v1.ManifestTypeKustomize)] = kustomizeManager
	helmfileManager := manager.NewHelmfile(k8sClient, opClient)
	m[string(v1.ManifestTypeHelmfile)] = helmfileManager
	// ORAS managerはartifact pull後にhelmfile/kustomize managerへ委譲する
	m[string(v1.ManifestTypeORAS)] = newORASManager(helmfileManager, kustomizeManager, orasOpts)

	// parallelマネージャーは、他のマネージャーをラップして並列実行を可能にする
	// そのため、他のマネージャーをすべて登録しておく必要がある
	m[string(v1.ManifestTypeParallel)] = manager.NewParallel(m)
	return m
}

// newORASManager は ORAS manager を組み立てて返します。
// docker config.json ベースの credential 解決に失敗した場合は anonymous fallback で続行します。
func newORASManager(helmfileManager, kustomizeManager orasmanager.DelegateManager, opts orasmanager.PullOptions) *orasmanager.ORAS {
	resolver, err := orasmanager.NewCredentialResolver()
	if err != nil {
		slog.Default().Debug("oras: failed to load docker credential store, falling back to anonymous",
			slog.String("error", err.Error()))
		resolver = orasmanager.NewCredentialResolverWithStore(nil)
	}
	factory := orasmanager.NewRemoteRepositoryFactory(resolver)
	puller := orasmanager.NewCachingPuller(factory)
	return orasmanager.NewWithOptions(puller, helmfileManager, kustomizeManager, opts)
}

func setupTestPlugins(k8sClient client.Client) map[string]testplugin.Plugin {
	m := make(map[string]testplugin.Plugin)
	m[string(v1.TestPluginTypeWaitUntil)] = testplugin.NewWaitUntil(k8sClient)
	m[string(v1.TestPluginTypeExistNonExist)] = testplugin.NewExistNonExist(k8sClient)
	return m
}

func (t *TazunaRunner) ConvertManifestPathFromCwd(baseDir string, tazuna *v1.Tazuna) {
	for mi := range tazuna.Spec.Manifests {
		manifestPathFromCwd := filepath.Join(baseDir, tazuna.Spec.Manifests[mi].Path)
		tazuna.Spec.Manifests[mi].Path = manifestPathFromCwd
	}
}

func WithTags(tags []string) RunnerOption {
	return func(r *TazunaRunner) {
		r.tags = tags
	}
}

// WithORASPullOptions は ORAS manager に渡す PullOptions を設定します。
// CLI フラグ (--no-cache / --offline) からの値を伝搬するために使います。
func WithORASPullOptions(opts orasmanager.PullOptions) RunnerOption {
	return func(r *TazunaRunner) {
		r.orasPullOpts = opts
	}
}
