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

// ApplyOptions は apply の挙動を切り替えるオプション群。
//
// Sync が true のとき、state を参照した差分適用モードになる。state を読んで Build()
// 出力との diff を取り、追加/変更のみを CreateOrUpdate する。差分ゼロのリソースは
// スキップする。
//
// Prune が true のとき、removed (state にあるが manifest に無い) リソースを Delete
// する。Sync が前提で、単独で指定された場合は呼び出し側でエラーにすること。
//
// Atomic が true のとき、Sync 時の state 保存を全 manifest 処理完了後にまとめて行う。
type ApplyOptions struct {
	Sync   bool
	Prune  bool
	Atomic bool
}

// TazunaRunner はTazuna操作を管理するオーバーオールな構造体
// これを用意し、各コマンドからはこの構造体を通して操作することで、
// Tazunaのロジックをそれぞれ拡張してビルドし、利用できるようにします
type TazunaRunner struct {
	logger       *slog.Logger
	tags         []string // マニフェストに適用するタグ
	k8sClient    client.Client
	opClient     op.Client // OnePasswordのクライアント
	orasPullOpts orasmanager.PullOptions
	applyOpts    ApplyOptions
	// managersOverride は主にテスト用途で manifest type -> Manager の差し替えを許容する。
	// nil の場合は setupManagers() で生成した本物のマネージャーが使われる。
	managersOverride map[string]manager.Manager
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

// ConvertManifestPathFromCwd は tazuna.yaml からの相対パスを cwd 起点のパスに
// 書き換えます。呼び出し元の Manifests スライスを破壊しないよう、専用の
// バッキング配列にコピーしてから書き換えます。Apply 等が Tazuna を値で受け取って
// いてもスライスヘッダはバッキング配列を共有するため、コピーしないと同じ
// Tazuna を二度渡すと baseDir が二重に prefix される問題が起きます。
func (t *TazunaRunner) ConvertManifestPathFromCwd(baseDir string, tazuna *v1.Tazuna) {
	if len(tazuna.Spec.Manifests) == 0 {
		return
	}
	copied := make([]v1.Manifest, len(tazuna.Spec.Manifests))
	copy(copied, tazuna.Spec.Manifests)
	for mi := range copied {
		copied[mi].Path = filepath.Join(baseDir, copied[mi].Path)
	}
	tazuna.Spec.Manifests = copied
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

// WithApplyOptions は apply の挙動オプション (--sync / --prune / --atomic) を設定します。
// 既存呼び出しを壊さないため、未指定時はゼロ値 (= 従来挙動) のままになります。
func WithApplyOptions(opts ApplyOptions) RunnerOption {
	return func(r *TazunaRunner) {
		r.applyOpts = opts
	}
}

// WithManagersOverride はテスト用にマネージャーマップを差し替えるオプションです。
// 本番コードからは使わない想定で、依存関係や並列実行の挙動を fake manager で
// 検証するためのフックを提供します。
func WithManagersOverride(managers map[string]manager.Manager) RunnerOption {
	return func(r *TazunaRunner) {
		r.managersOverride = managers
	}
}
