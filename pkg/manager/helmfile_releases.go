package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

// ReleaseInfo は helmfile.yaml で宣言された 1 つの release の要約情報です。
// `tazuna release list` の出力に利用します。
type ReleaseInfo struct {
	// Name は release 名 (helmfile.yaml の releases[].name)。
	Name string
	// Namespace は release の namespace。
	Namespace string
	// Chart は helmfile.yaml で宣言された chart 参照
	// (ローカルパス、`<alias>/<chart>`、`oci://...` のいずれか)。
	Chart string
	// Version は helmfile.yaml で宣言された chart のバージョン。
	Version string
	// HelmfilePath は解決された helmfile.yaml ファイルのパス
	// (manifest.Path がディレクトリのときは実ファイルまで解決したもの)。
	HelmfilePath string

	// chartSource は latest version の解決経路を判定するための内部情報。
	chartSource chartSource
}

// LatestVersionKey は同じ chart repository / chart 名を指す ReleaseInfo をまとめて
// 扱うためのキーを返します。ローカルチャートや repository を持たない参照では ""
// を返します (呼び出し側はキャッシュ対象から除外できます)。
func (r ReleaseInfo) LatestVersionKey() string {
	if r.chartSource.kind == "local" || r.chartSource.chartName == "" {
		return ""
	}
	return r.chartSource.kind + "|" + r.chartSource.repoURL + "|" + r.chartSource.chartName
}

// chartSource は release の chart がどの経路から取得されるかを表します。
type chartSource struct {
	// kind は "local" / "http" / "oci" のいずれか。
	kind      string
	chartName string
	repoURL   string
	username  string
	password  string
}

// ListReleases は helmfile 型 manifest を parse し、宣言された release の一覧を返します。
// chart の実 render や pull は行わないため、cluster / registry へのアクセスは発生しません。
// ただし helmfile.yaml.gotmpl の template render に vars を要するため、
// ConstructHelmfileVars と同じ経路で vars を解決します (1Password や env の参照が必要な
// 構成では対応する情報が揃っている必要があります)。
func (h *Helmfile) ListReleases(ctx context.Context, m v1.Manifest) ([]ReleaseInfo, error) {
	if m.Helmfile == nil {
		m.Helmfile = v1.DefaultHelmfile()
	}

	vars, err := h.ConstructHelmfileVars(ctx, &m)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to construct helmfile vars for manifest %s", m.Path)
	}

	helmfilePath, err := resolveHelmfilePath(m.Path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	raw, err := os.ReadFile(helmfilePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read helmfile %s", helmfilePath)
	}

	rendered, err := renderHelmfileTemplate(helmfilePath, raw, vars, h.environment)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to render helmfile template %s", helmfilePath)
	}

	var spec helmfileSpec
	if err := yaml.Unmarshal(rendered, &spec); err != nil {
		return nil, errors.Wrapf(err, "failed to parse helmfile %s", helmfilePath)
	}

	repos := make(map[string]helmfileRepository, len(spec.Repositories))
	for _, r := range spec.Repositories {
		repos[r.Name] = r
	}

	baseDir := filepath.Dir(helmfilePath)
	releases := make([]ReleaseInfo, 0, len(spec.Releases))
	for i := range spec.Releases {
		rel := &spec.Releases[i]
		info := ReleaseInfo{
			Name:         rel.Name,
			Namespace:    rel.Namespace,
			Chart:        rel.Chart,
			Version:      rel.Version,
			HelmfilePath: helmfilePath,
			chartSource:  resolveChartSource(baseDir, rel.Chart, repos),
		}
		releases = append(releases, info)
	}
	return releases, nil
}

// resolveChartSource は chart 参照から latest version 解決に必要な情報を返します。
// unknown な場合 (ローカルパス等) は kind="local" を返します。
func resolveChartSource(baseDir, chartRef string, repos map[string]helmfileRepository) chartSource {
	if registry.IsOCI(chartRef) {
		// oci://host/path/chartName の chartName を末尾から取り出す。
		trimmed := strings.TrimPrefix(chartRef, "oci://")
		trimmed = strings.TrimSuffix(trimmed, "/")
		lastSlash := strings.LastIndex(trimmed, "/")
		if lastSlash < 0 {
			return chartSource{kind: "local"}
		}
		return chartSource{
			kind:      "oci",
			chartName: trimmed[lastSlash+1:],
			repoURL:   "oci://" + trimmed[:lastSlash],
		}
	}

	if alias, chartName, ok := splitRepoAlias(chartRef); ok {
		if r, found := repos[alias]; found {
			if repositoryIsOCI(r) {
				base := strings.TrimSuffix(r.URL, "/")
				base = strings.TrimPrefix(base, "oci://")
				return chartSource{
					kind:      "oci",
					chartName: chartName,
					repoURL:   "oci://" + base,
					username:  r.Username,
					password:  r.Password,
				}
			}
			return chartSource{
				kind:      "http",
				chartName: chartName,
				repoURL:   r.URL,
				username:  r.Username,
				password:  r.Password,
			}
		}
	}

	_ = baseDir
	return chartSource{kind: "local"}
}

// LatestChartVersion は release の chart repository から最新バージョン文字列を返します。
// ローカルチャートなど repository から取得できない場合は ("", nil) を返します。
// registryClient は OCI chart 参照時に利用します。nil の場合は都度生成します。
func (h *Helmfile) LatestChartVersion(ctx context.Context, info ReleaseInfo, registryClient *registry.Client) (string, error) {
	switch info.chartSource.kind {
	case "http":
		return latestChartVersionFromHTTPRepo(info.chartSource)
	case "oci":
		rc := registryClient
		if rc == nil {
			c, err := registry.NewClient()
			if err != nil {
				return "", errors.Wrap(err, "failed to create helm registry client")
			}
			rc = c
		}
		return latestChartVersionFromOCI(rc, info.chartSource)
	default:
		return "", nil
	}
}

// latestChartVersionFromHTTPRepo は HTTP(S) helm repository の index.yaml から
// chartName の最新バージョンを返します。
func latestChartVersionFromHTTPRepo(src chartSource) (string, error) {
	settings := cli.New()
	entry := &repo.Entry{
		Name:     "tazuna-release-list-" + src.chartName,
		URL:      src.repoURL,
		Username: src.username,
		Password: src.password,
	}
	cr, err := repo.NewChartRepository(entry, getter.All(settings))
	if err != nil {
		return "", errors.WithStack(err)
	}
	// index.yaml を settings.RepositoryCache 配下にダウンロードする。
	if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
		return "", errors.WithStack(err)
	}
	cr.CachePath = settings.RepositoryCache
	indexPath, err := cr.DownloadIndexFile()
	if err != nil {
		return "", errors.Wrapf(err, "failed to download index for %s", src.repoURL)
	}
	idx, err := repo.LoadIndexFile(indexPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to load index file %s", indexPath)
	}
	// version を空文字にすると最新 (SortEntries 後の先頭) を返す。
	cv, err := idx.Get(src.chartName, "")
	if err != nil {
		return "", errors.Wrapf(err, "chart %s not found in repo %s", src.chartName, src.repoURL)
	}
	return cv.Version, nil
}

// latestChartVersionFromOCI は OCI registry の tags から最新の semver を返します。
func latestChartVersionFromOCI(rc *registry.Client, src chartSource) (string, error) {
	ref := strings.TrimPrefix(src.repoURL, "oci://") + "/" + src.chartName
	tags, err := rc.Tags(ref)
	if err != nil {
		return "", errors.Wrapf(err, "failed to list tags for %s", ref)
	}
	if len(tags) == 0 {
		return "", nil
	}
	// registry.Client.Tags は semver 降順で返すので先頭が最新。
	return tags[0], nil
}
