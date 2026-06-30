package manager

import (
	"bytes"
	"context"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"text/template"
	"time"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/hint"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/resource"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Masterminds/sprig/v3"
	"github.com/cockroachdb/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
//   - releases[].{name,namespace,chart,version,values}
//   - chart はローカルチャートへの相対パス、または oci:// で始まる OCI チャート参照。
//     OCI の場合は version を指定し、helm registry client 経由で pull する
//     (http(s) repository chart は未サポート)。
//   - values[] はファイルパス文字列、またはインライン map
//   - helmfile.yaml 自体の Go テンプレート (.StateValues / .Values) と sprig 関数
type Helmfile struct {
	client   client.Client
	opClient op.Client
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
		if err := h.waitForResources(ctx, logger, objects, m.Helmfile.TimeoutSeconds); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return objects, nil
}

// waitForResources は、指定されたリソースが Ready になるまで待機します
func (h *Helmfile) waitForResources(ctx context.Context, logger *slog.Logger, objects []client.Object, timeout int) error {
	// デフォルトのタイムアウトは 5 分
	if timeout == 0 {
		timeout = 300
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	for _, obj := range objects {
		if err := h.waitForResource(timeoutCtx, logger, obj); err != nil {
			return errors.Wrapf(err, "failed to wait for resource %s/%s", obj.GetNamespace(), obj.GetName())
		}
	}

	return nil
}

// waitForResource は、単一のリソースが Ready になるまで待機します
func (h *Helmfile) waitForResource(ctx context.Context, logger *slog.Logger, obj client.Object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	logger.InfoContext(ctx, "waiting for resource to be ready",
		slog.String("namespace", obj.GetNamespace()),
		slog.String("name", obj.GetName()),
		slog.String("kind", gvk.Kind))

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.Errorf("timeout waiting for %s %s/%s to be ready", gvk.Kind, obj.GetNamespace(), obj.GetName())
		case <-ticker.C:
			ready, err := h.isResourceReady(ctx, obj)
			if err != nil {
				return errors.WithStack(err)
			}
			if ready {
				logger.InfoContext(ctx, "resource is ready",
					slog.String("namespace", obj.GetNamespace()),
					slog.String("name", obj.GetName()),
					slog.String("kind", gvk.Kind))
				return nil
			}
		}
	}
}

// isResourceReady は、リソースが Ready 状態かどうかを確認します。
// 判定ロジック自体は pkg/resource に切り出されており、本メソッドはライブ取得した
// unstructured を resource.IsReady に委譲する薄いラッパーです。
func (h *Helmfile) isResourceReady(ctx context.Context, obj client.Object) (bool, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	key := client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	// リソースの最新状態を unstructured で取得
	// manifest.ConvertManifestsToObjects が *unstructured.Unstructured を返すため、
	// client.Get の結果も unstructured で受け取る
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(gvk)
	if err := h.client.Get(ctx, key, current); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// リソースがまだ存在しない場合は ready ではない
			return false, nil
		}
		return false, errors.WithStack(err)
	}

	return resource.IsReady(current)
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

			for _, field := range item.Fields {
				if v.Op.Key == v1.HelmFileVarOpKeyID {
					if field.ID == v.Op.Field {
						vars[k] = field.Value
						break
					}
				} else if v.Op.Key == v1.HelmFileVarOpKeyLabel {
					if field.Label == v.Op.Field {
						vars[k] = field.Value
						break
					}
				}
			}

			if vars[k] == "" {
				return nil, errors.Errorf("helmfile var %s has From op but op field %s not found in item %s", k, v.Op.Field, v.Op.Item)
			}
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
	return &Helmfile{client, opClient}
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
	rendered, err := renderHelmfileTemplate(helmfilePath, raw, vars)
	if err != nil {
		return "", errors.Wrapf(err, "failed to render helmfile template %s", helmfilePath)
	}

	var spec helmfileSpec
	if err := yaml.Unmarshal(rendered, &spec); err != nil {
		return "", errors.Wrapf(err, "failed to parse helmfile %s", helmfilePath)
	}

	baseDir := filepath.Dir(helmfilePath)

	var docs [][]byte
	for i := range spec.Releases {
		rel := spec.Releases[i]
		out, err := h.renderRelease(baseDir, &rel, m.Helmfile)
		if err != nil {
			return "", errors.Wrapf(err, "failed to render release %q", rel.Name)
		}
		if trimmed := bytes.TrimSpace([]byte(out)); len(trimmed) > 0 {
			docs = append(docs, trimmed)
		}
	}

	return string(bytes.Join(docs, []byte("\n---\n"))), nil
}

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
	Releases []helmfileRelease `json:"releases"`
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
func (h *Helmfile) renderRelease(baseDir string, rel *helmfileRelease, cfg *v1.ManifestHelmfile) (string, error) {
	if rel.Chart == "" {
		return "", errors.Errorf("release %q has no chart", rel.Name)
	}

	actionCfg := new(action.Configuration)
	actionCfg.Log = func(string, ...any) {}

	// OCI チャート (chart: oci://...) は helm registry client 経由で pull する必要が
	// あるため、actionCfg.RegistryClient を NewInstall 前に設定しておく
	// (action.NewInstall が ChartPathOptions.registryClient にコピーする)。
	if registry.IsOCI(rel.Chart) {
		registryClient, err := registry.NewClient()
		if err != nil {
			return "", errors.Wrap(err, "failed to create helm registry client")
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

	if cfg.KubeVersion != "" {
		kv, err := chartutil.ParseKubeVersion(cfg.KubeVersion)
		if err != nil {
			return "", errors.Wrapf(err, "invalid kubeVersion %q", cfg.KubeVersion)
		}
		inst.KubeVersion = kv
	}

	chartPath, err := h.resolveChartPath(inst, baseDir, rel.Chart)
	if err != nil {
		return "", errors.WithStack(err)
	}

	chrt, err := loader.Load(chartPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to load chart %s", chartPath)
	}

	// release の values (ファイル + インライン) を順にマージし、最後に
	// extraValueFiles を上書きとしてマージする (helmfile の --values 相当)。
	values := map[string]any{}
	for _, v := range rel.Values {
		merged, err := loadValueEntry(baseDir, v)
		if err != nil {
			return "", errors.Wrapf(err, "failed to load values for release %q", rel.Name)
		}
		values = mergeMaps(values, merged)
	}
	for _, vf := range cfg.ExtraValueFiles {
		merged, err := loadValueFile(baseDir, vf)
		if err != nil {
			return "", errors.Wrapf(err, "failed to load extra value file %s", vf)
		}
		values = mergeMaps(values, merged)
	}

	release, err := inst.Run(chrt, values)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return release.Manifest, nil
}

// resolveChartPath は chart 参照をローカルの chart パス (ディレクトリ or .tgz) に解決します。
//   - oci://... の場合は helm registry client で pull し、cache 上の .tgz パスを返す。
//   - それ以外はローカル chart とみなし、相対パスは baseDir 起点で解決する (従来挙動)。
func (h *Helmfile) resolveChartPath(inst *action.Install, baseDir, chartRef string) (string, error) {
	if registry.IsOCI(chartRef) {
		// LocateChart は OCI 参照を pull し、HELM_REPOSITORY_CACHE 配下の
		// .tgz への絶対パスを返す。version は inst.ChartPathOptions.Version を参照する。
		cp, err := inst.LocateChart(chartRef, cli.New())
		if err != nil {
			return "", errors.Wrapf(err, "failed to pull oci chart %s", chartRef)
		}
		return cp, nil
	}

	chartPath := chartRef
	if !filepath.IsAbs(chartPath) {
		chartPath = filepath.Join(baseDir, chartPath)
	}
	return chartPath, nil
}

// renderHelmfileTemplate は helmfile.yaml 本体を Go テンプレート + sprig で render します。
// helmfile 互換のため .StateValues と .Values の双方から vars を参照できるようにします。
func renderHelmfileTemplate(path string, raw []byte, vars map[string]any) ([]byte, error) {
	tmpl, err := template.New(filepath.Base(path)).
		Funcs(sprig.TxtFuncMap()).
		Option("missingkey=zero").
		Parse(string(raw))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	data := map[string]any{
		"StateValues": vars,
		"Values":      vars,
		"Environment": map[string]any{"Name": "default"},
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
