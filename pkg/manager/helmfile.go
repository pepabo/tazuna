package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/hint"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/resource"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Masterminds/sprig/v3"
	"github.com/cockroachdb/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

// Helmfile は helmfile (https://github.com/helmfile/helmfile) 形式の "サブセット" 互換
// レンダラです。helmfile 本体には依存せず、内部的に helm パッケージ
// (helm.sh/helm/v3) の in-memory render を用いて manifest を生成します。
//
// 設計上の意図 (TASKLIST 参照):
//   - helmfile.app.Template は結果を os.Stdout に書き出すため、従来は os.Stdout を
//     グローバルに差し替えてキャプチャしていた。これは並列 apply 時にレンダリング結果が
//     混線するバグの温床だった。helm の action.Install{ClientOnly,DryRun} は結果を
//     Go の値 (release.Manifest) として返すため、os.Stdout 差し替えが一切不要になる。
//   - helmfile.app は logger 未設定で segv するハックが必要だったが、helm では不要。
//
// インスピレーション元として helmfile をクレジットします。本実装が解釈する helmfile.yaml
// は以下のサブセットです (差分は docs/src/reference/manifest-types/helmfile.md を参照):
//   - repositories[].{name,url,username,password,oci}
//   - releases[].{name,namespace,chart,version,values}
//   - chart はローカルチャートへの相対パス、oci:// で始まる OCI チャート参照、または
//     <alias>/<chart> 形式のいずれか。<alias>/<chart> 形式は alias が repositories[] に
//     宣言されていれば、その url (HTTP(S) or OCI) から version の chart を pull する
//     (未宣言ならローカルパス扱い)。OCI は helm registry client 経由で pull する。
//   - values[] はファイルパス文字列、またはインライン map
//   - helmfile.yaml 自体の Go テンプレート (.StateValues / .Values) と sprig 関数
type Helmfile struct {
	client   client.Client
	opClient op.Client
	// environment は -e/--environment フラグの値。helmfile.yaml テンプレートの
	// {{ .Environment.Name }} に注入される。空文字なら "default"。
	environment string

	// restConfig は render 時に cluster から KubeVersion / APIVersions を discover
	// して helm の Capabilities に注入するために使う。nil の場合 (CI offline lint 等)
	// は helm SDK の DefaultCapabilities で render する (KubeVersion=v1.20.0 /
	// DefaultVersionSet)。cluster に接続できるコマンド (plan/apply/destroy/build)
	// では runner から WithRESTConfig で渡す。
	restConfig *rest.Config
	// discoveryOnce は plan/apply の 1 回の実行内で discovery 呼び出しを 1 回に抑える。
	discoveryOnce sync.Once
	cachedKubeVer *chartutil.KubeVersion
	cachedAPIs    chartutil.VersionSet
	discoveryErr  error
}

// Destroy implements Manager.
func (h *Helmfile) Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) (retErr error) {
	ctx, span := otel.Tracer(managerTracerName).Start(ctx, "Helmfile.Destroy",
		trace.WithAttributes(manifestSpanAttrs(m)...))
	defer func() {
		recordSpanError(span, retErr)
		span.End()
	}()

	objects, err := h.renderObjects(ctx, &m)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, obj := range objects {
		logger.DebugContext(ctx, "trying to delete an object", slog.String("namespace", obj.GetNamespace()), slog.String("name", obj.GetName()), slog.String("kind", obj.GetObjectKind().GroupVersionKind().Kind))
		if err := resource.DeleteObject(ctx, h.client, obj); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// Apply implements Manager.
func (h *Helmfile) Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) (objects []client.Object, retErr error) {
	ctx, span := otel.Tracer(managerTracerName).Start(ctx, "Helmfile.Apply",
		trace.WithAttributes(manifestSpanAttrs(m)...))
	defer func() {
		recordSpanError(span, retErr)
		span.End()
	}()

	// NOTE: helm の release は当てず、render した manifest を controller-runtime の
	//       client で apply する方針を維持する。これにより fake client を注入した
	//       ユニットテストが可能になり、また state 管理 (pkg/state) と組み合わせて
	//       kustomize / genesissecret と横断的に差分管理できる。helm rollback は
	//       利用できないが、クラスタ bootstrap という用途では不要とする。
	//
	// m.Helmfile の nil デフォルト補完は renderObjects -> render に集約しているため
	// ここでは行わない。renderObjects 呼び出し後は m.Helmfile が非 nil になる。
	objects, err := h.renderObjects(ctx, &m)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	span.SetAttributes(attribute.Int("manifest.objects", len(objects)))

	for _, obj := range objects {
		logger.DebugContext(ctx, "trying to create or update an object", slog.String("namespace", obj.GetNamespace()), slog.String("name", obj.GetName()), slog.String("kind", obj.GetObjectKind().GroupVersionKind().Kind))
		if err := resource.CreateOrUpdateForObject(ctx, h.client, obj); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// Wait が設定されている場合は、リソースが Ready になるまで待つ
	if m.Helmfile.Wait {
		if err := resource.WaitForReady(ctx, h.client, logger, objects, m.Helmfile.TimeoutSeconds); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return objects, nil
}

func (h *Helmfile) ConstructHelmfileVars(ctx context.Context, m *v1.Manifest) (map[string]any, error) {
	vars := make(map[string]any)

	for k, v := range m.Helmfile.Vars {
		if v.From == "" {
			return nil, errors.Errorf("helmfile var %s has no From field, supported From is 'env/static/op'", k)
		}

		switch v.From {
		case v1.HelmFileVarFromEnv:
			if v.Env == nil {
				return nil, errors.Errorf("helmfile var %s has From env but no env field", k)
			}
			env, ok := os.LookupEnv(*v.Env)
			if !ok {
				return nil, errors.Errorf("helmfile var %s has From env but environment variable %s is not set", k, *v.Env)
			}
			vars[k] = env
		case v1.HelmFileVarFromStatic:
			// 実行時の安全性のためのvalidation（通常は ValidateTazuna() で防がれる）
			count := 0
			if v.Static != nil {
				count++
			}
			if v.StaticSlice != nil {
				count++
			}
			if v.StaticMap != nil {
				count++
			}
			if count == 0 {
				return nil, errors.Errorf("helmfile var %s has From static but no static/staticSlice/staticMap field", k)
			}
			if count > 1 {
				return nil, errors.Errorf("helmfile var %s has From static but multiple static fields are set", k)
			}

			if v.Static != nil {
				vars[k] = *v.Static
			} else if v.StaticSlice != nil {
				vars[k] = v.StaticSlice
			} else if v.StaticMap != nil {
				vars[k] = v.StaticMap
			}
		case v1.HelmFileVarFromOp:
			if h.opClient == nil {
				return nil, errors.New("helmfile var has From op but OnePassword client is not set")
			}
			if v.Op == nil {
				return nil, errors.Errorf("helmfile var %s has From op but no op field", k)
			}

			if v.Op.Key == "" || v.Op.Vault == "" || v.Op.Item == "" || v.Op.Field == "" {
				return nil, errors.Errorf("helmfile var %s has From op but op field is not set properly", k)
			}

			item, err := h.opClient.GetVaultItem(ctx, v.Op.Vault, v.Op.Item)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get vault item %s from %s", v.Op.Item, v.Op.Vault)
			}

			found := false
			for _, field := range item.Fields {
				if v.Op.Key == v1.HelmFileVarOpKeyID {
					if field.ID == v.Op.Field {
						vars[k] = field.Value
						found = true
						break
					}
				} else if v.Op.Key == v1.HelmFileVarOpKeyLabel {
					if field.Label == v.Op.Field {
						vars[k] = field.Value
						found = true
						break
					}
				}
			}

			if !found {
				return nil, errors.Errorf("helmfile var %s has From op but op field %s not found in item %s", k, v.Op.Field, v.Op.Item)
			}
		default:
			// typo等の未知のFromを黙って無視するとvarが欠落したまま
			// missingkey=zeroでゼロ値レンダリングされるため、明示的にエラーにする
			return nil, errors.Errorf("helmfile var %s has unsupported From field: %s, supported From is 'env/static/op'", k, v.From)
		}
	}

	// hint処理: tazuna.hint.yamlが存在すれば検証・デフォルト注入を行う
	hintDir := filepath.Dir(m.Path)
	hintFile, err := hint.LoadHintFile(hintDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load hint file from %s", hintDir)
	}
	if hintFile != nil {
		if err := hint.ValidateHint(hintFile); err != nil {
			return nil, errors.Wrap(err, "invalid hint file")
		}
		if err := hint.ValidateVarsAgainstHint(hintFile, m.Helmfile.Vars); err != nil {
			return nil, errors.Wrap(err, "vars validation against hint failed")
		}
		vars, err = hint.MergeVarsWithHint(hintFile, vars)
		if err != nil {
			return nil, errors.Wrap(err, "failed to merge vars with hint")
		}
	}

	return vars, nil
}

var _ Manager = &Helmfile{}

func NewHelmfile(client client.Client, opClient op.Client) *Helmfile {
	return &Helmfile{client: client, opClient: opClient}
}

// WithEnvironment は -e/--environment フラグの値を設定します。
// helmfile.yaml テンプレートの {{ .Environment.Name }} に反映されます。
func (h *Helmfile) WithEnvironment(environment string) *Helmfile {
	h.environment = environment
	return h
}

// WithRESTConfig は render 時に cluster から Capabilities (KubeVersion / APIVersions)
// を discover するための rest.Config を設定します。nil の場合は helm SDK の
// DefaultCapabilities で render されるため、k8s 1.22+ で削除された古い GVK
// (例: policy/v1beta1 PodDisruptionBudget) が render されるチャートで plan/apply が
// 失敗します。cluster に接続できるコマンドでは必ず設定してください。
func (h *Helmfile) WithRESTConfig(cfg *rest.Config) *Helmfile {
	h.restConfig = cfg
	return h
}

// clusterCapabilities は cluster から KubeVersion と APIVersions を取得して返します。
// restConfig が nil、あるいは discovery に失敗した場合は (nil, nil, err) を返し、
// 呼び出し側は helm SDK の DefaultCapabilities にフォールバックします。
// 結果は sync.Once でキャッシュされ、1 回の render 全体で 1 回の discovery で済みます。
func (h *Helmfile) clusterCapabilities() (*chartutil.KubeVersion, chartutil.VersionSet, error) {
	h.discoveryOnce.Do(func() {
		if h.restConfig == nil {
			return
		}
		dc, err := discovery.NewDiscoveryClientForConfig(h.restConfig)
		if err != nil {
			h.discoveryErr = errors.Wrap(err, "failed to create discovery client")
			return
		}
		sv, err := dc.ServerVersion()
		if err != nil {
			h.discoveryErr = errors.Wrap(err, "failed to get server version")
			return
		}
		kv, err := chartutil.ParseKubeVersion(sv.GitVersion)
		if err != nil {
			h.discoveryErr = errors.Wrapf(err, "invalid server version %q", sv.GitVersion)
			return
		}
		h.cachedKubeVer = kv

		// helm の DefaultVersionSet (kube 標準の core GVK 一式) をベースに、cluster に
		// 実在する GroupVersion と GroupVersion/Kind を追加していく。
		// karpenter chart のように templates 側が Capabilities.APIVersions.Has "policy/v1"
		// を条件分岐する場合はこれで v1 分岐が選ばれる。
		apis := chartutil.DefaultVersionSet
		groups, resources, err := dc.ServerGroupsAndResources()
		// ServerGroupsAndResources は一部 API が unavailable でも部分成功で
		// groups/resources を返すことがある (helm 自身も同様に部分結果を採用する)。
		// discovery エラーそのものは fatal にはせず、取れた分だけ APIVersions に反映する。
		_ = err
		for _, g := range groups {
			for _, v := range g.Versions {
				apis = append(apis, v.GroupVersion)
			}
		}
		for _, rl := range resources {
			apis = append(apis, rl.GroupVersion)
			for _, r := range rl.APIResources {
				apis = append(apis, rl.GroupVersion+"/"+r.Kind)
			}
		}
		h.cachedAPIs = apis
	})
	return h.cachedKubeVer, h.cachedAPIs, h.discoveryErr
}

// Build implements Manager.
func (h *Helmfile) Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (result string, retErr error) {
	ctx, span := otel.Tracer(managerTracerName).Start(ctx, "Helmfile.Build",
		trace.WithAttributes(manifestSpanAttrs(m)...))
	defer func() {
		recordSpanError(span, retErr)
		span.End()
	}()

	// m.Helmfile の nil デフォルト補完は render に集約している。
	return h.render(ctx, &m)
}

// renderObjects は helmfile をレンダリングして client.Object 群へ変換します。
// Apply / Destroy が共通で利用します。
func (h *Helmfile) renderObjects(ctx context.Context, m *v1.Manifest) ([]client.Object, error) {
	// m.Helmfile の nil デフォルト補完は render が行うため、render 後は非 nil。
	out, err := h.render(ctx, m)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	objects, err := manifest.ConvertManifestsToObjects([]byte(out), m.Helmfile.DefaultNamespace)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return objects, nil
}

// render は helmfile.yaml (サブセット) を解釈し、各 release を helm の in-memory
// render に通して結合した manifest YAML 文字列を返します。os.Stdout には一切触れません。
func (h *Helmfile) render(ctx context.Context, m *v1.Manifest) (string, error) {
	if m.Helmfile == nil {
		m.Helmfile = v1.DefaultHelmfile()
	}

	vars, err := h.ConstructHelmfileVars(ctx, m)
	if err != nil {
		return "", errors.Wrapf(err, "failed to construct helmfile vars for manifest %s", m.Path)
	}

	// m.Path はファイル (helmfile.yaml) でもディレクトリでも良い。ディレクトリの場合は
	// helmfile のデフォルト探索順でファイルを解決する (後方互換)。
	helmfilePath, err := resolveHelmfilePath(m.Path)
	if err != nil {
		return "", errors.WithStack(err)
	}

	raw, err := os.ReadFile(helmfilePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read helmfile %s", helmfilePath)
	}

	// helmfile.yaml 自体を Go テンプレートとして解釈する (.StateValues / .Values)。
	rendered, err := renderHelmfileTemplate(helmfilePath, raw, vars, h.environment)
	if err != nil {
		return "", errors.Wrapf(err, "failed to render helmfile template %s", helmfilePath)
	}

	var spec helmfileSpec
	if err := yaml.Unmarshal(rendered, &spec); err != nil {
		return "", errors.Wrapf(err, "failed to parse helmfile %s", helmfilePath)
	}

	baseDir := filepath.Dir(helmfilePath)

	// repositories[] を alias→repository の map に変換する。release の chart が
	// `<alias>/<chart>` 形式のときにリモート chart として解決するために使う。
	repos := make(map[string]helmfileRepository, len(spec.Repositories))
	for _, r := range spec.Repositories {
		repos[r.Name] = r
	}

	// ExtraValueFiles は release 間で共通のため、release ごとに再読込せず
	// 一度だけ読み込んで共有する。
	extraValues := map[string]any{}
	for _, vf := range m.Helmfile.ExtraValueFiles {
		merged, err := loadValueFile(baseDir, vf)
		if err != nil {
			return "", errors.Wrapf(err, "failed to load extra value file %s", vf)
		}
		extraValues = mergeMaps(extraValues, merged)
	}

	// OCI chart 用の registry client は release ごとに生成せず共有する。
	// chart: oci://... の直接参照に加え、repositories[] で宣言された OCI repository の
	// `<alias>/<chart>` 参照も対象にする。
	var registryClient *registry.Client
	for i := range spec.Releases {
		if releaseNeedsOCI(&spec.Releases[i], repos) {
			rc, err := registry.NewClient()
			if err != nil {
				return "", errors.Wrap(err, "failed to create helm registry client")
			}
			registryClient = rc
			break
		}
	}

	// release は互いに独立に render できるため errgroup で並列化する。
	results := make([]string, len(spec.Releases))
	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(releaseRenderConcurrency)
	for i := range spec.Releases {
		g.Go(func() error {
			rel := &spec.Releases[i]
			out, err := h.renderRelease(baseDir, rel, m.Helmfile, extraValues, repos, registryClient)
			if err != nil {
				return errors.Wrapf(err, "failed to render release %q", rel.Name)
			}
			results[i] = out
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return "", err
	}

	var docs [][]byte
	for _, out := range results {
		if trimmed := bytes.TrimSpace([]byte(out)); len(trimmed) > 0 {
			docs = append(docs, trimmed)
		}
	}

	return string(bytes.Join(docs, []byte("\n---\n"))), nil
}

// releaseRenderConcurrency は release render の並列数。helm の in-memory render は
// CPU バウンドのため控えめな値にする。
const releaseRenderConcurrency = 4

// defaultHelmfileNames は m.Path がディレクトリのときに探索する helmfile ファイル名の
// 優先順位。helmfile 本体の既定 (gotmpl 優先) に倣う。
var defaultHelmfileNames = []string{
	"helmfile.yaml.gotmpl",
	"helmfile.yaml",
	"helmfile.yml.gotmpl",
	"helmfile.yml",
}

// resolveHelmfilePath は m.Path がファイルならそのまま、ディレクトリなら既定名で
// helmfile ファイルを探索して返す。
func resolveHelmfilePath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to stat helmfile path %s", path)
	}
	if !info.IsDir() {
		return path, nil
	}
	for _, name := range defaultHelmfileNames {
		candidate := filepath.Join(path, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.Errorf("no helmfile found in directory %s (looked for %v)", path, defaultHelmfileNames)
}

// helmfileSpec は解釈可能な helmfile.yaml のサブセットです。
type helmfileSpec struct {
	Repositories []helmfileRepository `json:"repositories"`
	Releases     []helmfileRelease    `json:"releases"`
}

// helmfileRepository は helmfile.yaml の repositories[] エントリ (サブセット) です。
// release の chart が `<name>/<chart>` 形式のとき、name に一致する repository の
// url から chart を pull します。HTTP(S) と OCI (oci:// もしくは oci: true) の
// 双方に対応します。
type helmfileRepository struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	// OCI は url が oci スキームでない場合でも OCI registry として扱うためのフラグです
	// (helmfile 互換)。url が oci:// で始まる場合は自動的に OCI と判定します。
	OCI bool `json:"oci"`
}

// helmfileRelease は 1 つの helm release 宣言です。
type helmfileRelease struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Chart     string `json:"chart"`
	Version   string `json:"version"`
	// Values はファイルパス文字列、またはインライン map のリストです。
	Values []any `json:"values"`
}

// renderRelease は 1 release を helm の ClientOnly + DryRun install で render します。
// extraValues は release 間で共有される追加 values (読み込み済み)、repos は
// repositories[] の alias→repository map、registryClient は OCI chart 用の共有 client
// (OCI chart がない場合は nil)。
func (h *Helmfile) renderRelease(baseDir string, rel *helmfileRelease, cfg *v1.ManifestHelmfile, extraValues map[string]any, repos map[string]helmfileRepository, registryClient *registry.Client) (string, error) {
	if rel.Chart == "" {
		return "", errors.Errorf("release %q has no chart", rel.Name)
	}

	actionCfg := new(action.Configuration)
	actionCfg.Log = func(string, ...any) {}

	// OCI チャート (chart: oci://... もしくは OCI repository の <alias>/<chart>) は
	// helm registry client 経由で pull する必要があるため、actionCfg.RegistryClient を
	// NewInstall 前に設定しておく (action.NewInstall が ChartPathOptions.registryClient に
	// コピーする)。
	if releaseNeedsOCI(rel, repos) {
		if registryClient == nil {
			return "", errors.Errorf("release %q references an OCI chart but no registry client is available", rel.Name)
		}
		actionCfg.RegistryClient = registryClient
	}

	inst := action.NewInstall(actionCfg)
	inst.ClientOnly = true
	inst.DryRun = true
	// Replace により dry-run 時の release 名重複チェックを無効化する (再現性のため)。
	inst.Replace = true
	inst.ReleaseName = rel.Name
	inst.Namespace = rel.Namespace
	inst.IncludeCRDs = cfg.IncludeCRDs
	inst.Version = rel.Version

	// Capabilities (KubeVersion / APIVersions) の設定順:
	//   1. cfg.KubeVersion が明示指定されていれば従来通りそれを使う (offline lint 互換)。
	//   2. そうでなく restConfig が設定されていれば cluster から discover する。
	//      discover で得た KubeVersion / APIVersions の両方を注入することで、
	//      chart 内の {{ .Capabilities.APIVersions.Has "policy/v1" }} 分岐等が
	//      実 cluster と一致した GVK で render される。
	//   3. どちらも無ければ helm SDK の DefaultCapabilities に委ねる。
	if cfg.KubeVersion != "" {
		kv, err := chartutil.ParseKubeVersion(cfg.KubeVersion)
		if err != nil {
			return "", errors.Wrapf(err, "invalid kubeVersion %q", cfg.KubeVersion)
		}
		inst.KubeVersion = kv
	} else if kv, apis, err := h.clusterCapabilities(); err == nil && kv != nil {
		inst.KubeVersion = kv
		inst.APIVersions = apis
	}

	chartPath, err := h.resolveChartPath(inst, baseDir, rel.Chart, repos, registryClient)
	if err != nil {
		return "", errors.WithStack(err)
	}

	chrt, err := loader.Load(chartPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to load chart %s", chartPath)
	}

	// release の values (ファイル + インライン) を順にマージし、最後に
	// extraValues (extraValueFiles の読み込み済みマージ結果) を上書きとして
	// マージする (helmfile の --values 相当)。
	values := map[string]any{}
	for _, v := range rel.Values {
		merged, err := loadValueEntry(baseDir, v)
		if err != nil {
			return "", errors.Wrapf(err, "failed to load values for release %q", rel.Name)
		}
		values = mergeMaps(values, merged)
	}
	values = mergeMaps(values, extraValues)

	release, err := inst.Run(chrt, values)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return release.Manifest, nil
}

// resolveChartPath は chart 参照をローカルの chart パス (ディレクトリ or .tgz) に解決します。
//   - oci://... の場合は helm registry client で pull し、cache 上の .tgz パスを返す。
//   - `<alias>/<chart>` 形式で alias が repositories[] に宣言されている場合は、その
//     repository (HTTP(S) or OCI) から chart を pull し、cache 上の .tgz パスを返す。
//   - それ以外はローカル chart とみなし、相対パスは baseDir 起点で解決する (従来挙動)。
func (h *Helmfile) resolveChartPath(inst *action.Install, baseDir, chartRef string, repos map[string]helmfileRepository, registryClient *registry.Client) (string, error) {
	if registry.IsOCI(chartRef) {
		// LocateChart は OCI 参照を pull し、HELM_REPOSITORY_CACHE 配下の
		// .tgz への絶対パスを返す。version は inst.ChartPathOptions.Version を参照する。
		cp, err := inst.LocateChart(chartRef, cli.New())
		if err != nil {
			return "", errors.Wrapf(err, "failed to pull oci chart %s", chartRef)
		}
		return cp, nil
	}

	// `<alias>/<chart>` 形式で alias が repositories[] に宣言されていれば、
	// リモート repository から pull する。宣言がなければローカルパスとして扱う
	// (後方互換: 既存のローカル chart 相対パスを壊さない)。
	if alias, chartName, ok := splitRepoAlias(chartRef); ok {
		if repo, found := repos[alias]; found {
			return h.resolveRepoChart(inst, repo, chartName, registryClient)
		}
	}

	chartPath := chartRef
	if !filepath.IsAbs(chartPath) {
		chartPath = filepath.Join(baseDir, chartPath)
	}
	return chartPath, nil
}

// resolveRepoChart は repositories[] で宣言された HTTP(S) / OCI repository から
// chartName を pull し、ローカルの .tgz への絶対パスを返します。
//
// HTTP(S) repository については action.ChartPathOptions.LocateChart をそのまま
// 使わず、下記の findAndDownloadRepoChart で同等のロジックを直接呼び出します。
// LocateChart は「cwd に chartName と同名のファイル/ディレクトリが存在すれば
// repository を無視してそれをローカル chart として扱う」という互換動作
// (helm issue #7862) を内蔵しており、alias で明示的に repository を指定して
// いるにもかかわらず、tazuna の実行時カレントディレクトリ (build 対象の
// manifests リポジトリ) にたまたま同名のディレクトリが存在するだけで
// 誤って解決されてしまう。ここは repositories[] の宣言によって repository
// からの取得だと確定しているため、cwd の内容に左右されてはならない。
func (h *Helmfile) resolveRepoChart(inst *action.Install, hfRepo helmfileRepository, chartName string, registryClient *registry.Client) (string, error) {
	if repositoryIsOCI(hfRepo) {
		if registryClient == nil {
			return "", errors.Errorf("repository %q is an OCI registry but no registry client is available", hfRepo.Name)
		}
		ref := ociChartRef(hfRepo.URL, chartName)
		cp, err := inst.LocateChart(ref, cli.New())
		if err != nil {
			return "", errors.Wrapf(err, "failed to pull oci chart %s from repository %q", chartName, hfRepo.Name)
		}
		return cp, nil
	}

	cp, err := findAndDownloadRepoChart(hfRepo, chartName, inst.Version, cli.New())
	if err != nil {
		return "", errors.Wrapf(err, "failed to pull chart %s from repository %q (%s)", chartName, hfRepo.Name, hfRepo.URL)
	}
	return cp, nil
}

// findAndDownloadRepoChart は HTTP(S) repository の index.yaml から chartName の
// tarball URL を解決し、settings.RepositoryCache 配下へ download します。
// helm.sh/helm/v3/pkg/action.ChartPathOptions.LocateChart の HTTP(S) 分岐と
// 同等の処理ですが、同関数が行うカレントディレクトリの os.Stat チェック
// (repository 宣言よりローカル同名ファイルを優先する挙動) は意図的に行いません。
func findAndDownloadRepoChart(hfRepo helmfileRepository, chartName, version string, settings *cli.EnvSettings) (string, error) {
	chartURL, err := repo.FindChartInAuthRepoURL(hfRepo.URL, hfRepo.Username, hfRepo.Password, chartName, strings.TrimSpace(version), "", "", "", getter.All(settings))
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
		return "", errors.WithStack(err)
	}

	dl := downloader.ChartDownloader{
		Out:              io.Discard,
		Getters:          getter.All(settings),
		Options:          []getter.Option{getter.WithBasicAuth(hfRepo.Username, hfRepo.Password)},
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
	}

	dest, _, err := dl.DownloadTo(chartURL, strings.TrimSpace(version), settings.RepositoryCache)
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(dest)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return abs, nil
}

// releaseNeedsOCI は release の chart が OCI registry 経由の pull を要するかを返します。
// chart: oci://... の直接参照、または OCI repository の `<alias>/<chart>` 参照が該当します。
func releaseNeedsOCI(rel *helmfileRelease, repos map[string]helmfileRepository) bool {
	if registry.IsOCI(rel.Chart) {
		return true
	}
	if alias, _, ok := splitRepoAlias(rel.Chart); ok {
		if repo, found := repos[alias]; found && repositoryIsOCI(repo) {
			return true
		}
	}
	return false
}

// splitRepoAlias は chart 参照が `<alias>/<chart>` 形式かを判定し、alias と chart 名を
// 返します。oci:// などのスキーム付き参照、絶対パス、"." 始まりの明示的なローカルパス、
// および chart 名に "/" を含む深いローカルパスは対象外とします (ok=false)。
func splitRepoAlias(chartRef string) (alias, chart string, ok bool) {
	if strings.Contains(chartRef, "://") || filepath.IsAbs(chartRef) || strings.HasPrefix(chartRef, ".") {
		return "", "", false
	}
	alias, chart, found := strings.Cut(chartRef, "/")
	if !found || alias == "" || chart == "" || strings.Contains(chart, "/") {
		return "", "", false
	}
	return alias, chart, true
}

// repositoryIsOCI は repository が OCI registry かを返します。
func repositoryIsOCI(repo helmfileRepository) bool {
	return repo.OCI || registry.IsOCI(repo.URL)
}

// ociChartRef は repository の url と chart 名から oci:// 参照を組み立てます。
// url が oci:// で始まらない場合 (oci: true 指定時) も oci:// を前置します。
func ociChartRef(repoURL, chartName string) string {
	base := strings.TrimSuffix(repoURL, "/")
	base = strings.TrimPrefix(base, "oci://")
	return "oci://" + base + "/" + chartName
}

// helmfileTemplateFuncs は helmfile render 用の sprig FuncMap を返します。
// env / expandenv は除外します。ORAS 経由で取得したリモートアーティファクト内の
// helmfile もこの経路で render されるため、悪意ある（または改竄された）テンプレートが
// `{{ env "AWS_SECRET_ACCESS_KEY" }}` のように実行者の環境変数を窃取して
// マニフェストへ焼き込むのを防ぐ。環境変数を参照したい場合は helmfile vars の
// `from: env` を明示的に使うこと。
//
// sprig 自体には toYaml/fromYaml/toJson/fromJson が含まれていない。本家 helmfile や
// helm chart テンプレートはこれらを追加で提供しており、一部の helmfile.yaml.gotmpl は
// `{{ .StateValues.xxx | toYaml | indent N }}` のようにリスト/マップ値を埋め込むために
// これらの関数へ依存している。互換性のため helm.sh/helm/v3/pkg/engine の funcMap と
// 同じ実装を追加する。
func helmfileTemplateFuncs() template.FuncMap {
	funcs := sprig.TxtFuncMap()
	delete(funcs, "env")
	delete(funcs, "expandenv")

	funcs["toYaml"] = toYAML
	funcs["fromYaml"] = fromYAML
	funcs["toJson"] = toJSON
	funcs["fromJson"] = fromJSON

	return funcs
}

// toYAML は v を YAML表現の文字列に変換します。helm chart テンプレートの toYaml と
// 同様、marshal に失敗した場合は空文字列を返します (テンプレート内で扱いやすくするため)。
func toYAML(v any) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(string(data), "\n")
}

// fromYAML は YAML文字列を map にデコードします。失敗した場合は "Error" キーに
// エラーメッセージを詰めて返します (helm chart テンプレートの fromYaml と同じ挙動)。
func fromYAML(str string) map[string]any {
	m := map[string]any{}
	if err := yaml.Unmarshal([]byte(str), &m); err != nil {
		m["Error"] = err.Error()
	}
	return m
}

// toJSON は v を JSON表現の文字列に変換します。
func toJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

// fromJSON は JSON文字列を map にデコードします。失敗した場合は "Error" キーに
// エラーメッセージを詰めて返します。
func fromJSON(str string) map[string]any {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(str), &m); err != nil {
		m["Error"] = err.Error()
	}
	return m
}

// renderHelmfileTemplate は helmfile.yaml 本体を Go テンプレート + sprig で render します。
// helmfile 互換のため .StateValues と .Values の双方から vars を参照できるようにします。
// environment は {{ .Environment.Name }} に注入される。空文字なら helmfile の
// 慣習に合わせて "default" になる。
func renderHelmfileTemplate(path string, raw []byte, vars map[string]any, environment string) ([]byte, error) {
	tmpl, err := template.New(filepath.Base(path)).
		Funcs(helmfileTemplateFuncs()).
		Option("missingkey=zero").
		Parse(string(raw))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if environment == "" {
		environment = "default"
	}
	data := map[string]any{
		"StateValues": vars,
		"Values":      vars,
		"Environment": map[string]any{"Name": environment},
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, errors.WithStack(err)
	}
	return buf.Bytes(), nil
}

// loadValueEntry は helmfile release の values[] の 1 エントリを map に変換します。
// 文字列ならファイルパスとして読み込み、map ならそのまま採用します。
func loadValueEntry(baseDir string, entry any) (map[string]any, error) {
	switch v := entry.(type) {
	case string:
		return loadValueFile(baseDir, v)
	case map[string]any:
		return v, nil
	case nil:
		return map[string]any{}, nil
	default:
		return nil, errors.Errorf("unsupported values entry type %T", entry)
	}
}

// loadValueFile は value ファイルを読み込み map に変換します。
func loadValueFile(baseDir, path string) (map[string]any, error) {
	p := path
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read value file %s", p)
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, errors.Wrapf(err, "failed to parse value file %s", p)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// mergeMaps は b を a に深くマージします。衝突時は b が優先されます
// (helm CLI の cli/values.mergeMaps と同一セマンティクス)。
func mergeMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a))
	maps.Copy(out, a)
	for k, v := range b {
		if vMap, ok := v.(map[string]any); ok {
			if existing, ok := out[k]; ok {
				if existingMap, ok := existing.(map[string]any); ok {
					out[k] = mergeMaps(existingMap, vMap)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}
