package runner

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/cockroachdb/errors"
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
	// providersBaseDir は ProviderConfig 内の相対 path (envfile.path 等) を解決する
	// ためのディレクトリ。通常は tazuna.yaml のディレクトリで、Apply/Build/Destroy/
	// StateDiff の入口で設定される。直接 ApplyToCluster 等を呼ぶテストでは空のままで
	// 構わない (envfile provider を使わない限り影響しない)。
	providersBaseDir string
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

// setupManagers は manifest type -> Manager のマップを組み立てる。
// providers にはユーザ宣言の Secret provider 一覧を渡す。registry には組み込みの
// "default-op" provider が opClient ありの場合に自動登録されるため、tazuna.yaml で
// providers を一切宣言しなくても従来挙動 (1Password のみ利用可能) が保たれる。
// providers の baseDir は registry 構築時に envfile path 解決のために使う。
func setupManagers(
	k8sClient client.Client,
	opClient op.Client,
	orasOpts orasmanager.PullOptions,
	providers []v1.ProviderConfig,
	providersBaseDir string,
) (map[string]manager.Manager, error) {
	registry, err := buildProviderRegistry(opClient, providers, providersBaseDir)
	if err != nil {
		return nil, err
	}

	m := make(map[string]manager.Manager)
	m[string(v1.ManifestTypeGenesisSecret)] = manager.NewGenesisSecret(k8sClient, registry)
	kustomizeManager := manager.NewKustomize(k8sClient)
	m[string(v1.ManifestTypeKustomize)] = kustomizeManager
	helmfileManager := manager.NewHelmfile(k8sClient, opClient)
	m[string(v1.ManifestTypeHelmfile)] = helmfileManager
	// ORAS managerはartifact pull後にhelmfile/kustomize managerへ委譲する
	m[string(v1.ManifestTypeORAS)] = newORASManager(helmfileManager, kustomizeManager, orasOpts)

	// parallelマネージャーは、他のマネージャーをラップして並列実行を可能にする
	// そのため、他のマネージャーをすべて登録しておく必要がある
	m[string(v1.ManifestTypeParallel)] = manager.NewParallel(m)
	return m, nil
}

// buildProviderRegistry は Secret provider レジストリを構築する。
// opClient が nil でなければ "default-op" が登録され、その後ユーザ宣言の
// providers[] をパースして個別 provider を登録する。providersBaseDir は
// envfile provider の相対 path 解決に使う (tazuna.yaml のディレクトリ)。
func buildProviderRegistry(opClient op.Client, providers []v1.ProviderConfig, providersBaseDir string) (*genesissecret.ProviderRegistry, error) {
	registry := genesissecret.NewProviderRegistry()

	// 組み込みの default 1Password provider を登録する。opClient が nil でも
	// (Fetch が呼ばれない構成 = secrets が空の manifest など) 登録だけはしておく
	// ことで、`.spec.provider` 未指定 manifest の "default-op" 解決を維持する。
	// 実際に Fetch を呼ぶ secrets を持つ manifest で opClient が nil の場合は
	// 既存挙動と同じく Fetch 内で nil deref / 空 client エラーになる。
	if err := registry.Register(v1.DefaultOnePasswordProviderName, genesissecret.NewOnePasswordProvider(opClient)); err != nil {
		return nil, errors.Wrap(err, "failed to register default 1Password provider")
	}

	for i, pc := range providers {
		if err := registerProvider(registry, pc, providersBaseDir); err != nil {
			return nil, errors.Wrapf(err, "failed to register provider[%d] %q", i, pc.Name)
		}
	}

	return registry, nil
}

// registerProvider は 1 つの ProviderConfig を registry に登録する。
// 型ごとの分岐と必須フィールド検査をここで集約する。
func registerProvider(registry *genesissecret.ProviderRegistry, pc v1.ProviderConfig, baseDir string) error {
	if pc.Name == "" {
		return fmt.Errorf("provider name must not be empty")
	}
	if pc.Name == v1.DefaultOnePasswordProviderName {
		return fmt.Errorf("provider name %q is reserved", v1.DefaultOnePasswordProviderName)
	}

	switch pc.Type {
	case v1.ProviderTypeOnePassword:
		// 現状 OnePassword provider はユーザ定義経路でも default と同じ opClient を
		// 流用するため、別名で複数登録する用途以外の利用は想定していない。
		return fmt.Errorf("provider %q: explicit %q type is not supported yet; use the built-in %q instead",
			pc.Name, v1.ProviderTypeOnePassword, v1.DefaultOnePasswordProviderName)
	case v1.ProviderTypeEnvFile:
		if pc.EnvFile == nil || pc.EnvFile.Path == "" {
			return fmt.Errorf("provider %q (envfile): envfile.path is required", pc.Name)
		}
		// 相対 path は tazuna.yaml のディレクトリを基準に解決する
		path := pc.EnvFile.Path
		if !filepath.IsAbs(path) && baseDir != "" {
			path = filepath.Join(baseDir, path)
		}
		return registry.Register(pc.Name, genesissecret.NewEnvFileProvider(path))
	case "":
		return fmt.Errorf("provider %q: type is required", pc.Name)
	default:
		return fmt.Errorf("provider %q: unsupported type %q", pc.Name, pc.Type)
	}
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
