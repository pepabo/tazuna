package runner

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"golang.org/x/sync/errgroup"
	"helm.sh/helm/v3/pkg/registry"
)

// ChartListItem は `tazuna chart list` の 1 行を表します。
type ChartListItem struct {
	// ManifestName は tazuna.yaml で宣言された manifest の名前。
	ManifestName string
	// Chart は helmfile.yaml で宣言された chart 参照
	// (ローカルパス、`<alias>/<chart>`、`oci://...` のいずれか)。
	Chart string
	// Release は chart をインストールする helm release 名。
	Release string
	// FilePath は release が定義されている helmfile.yaml のパス。
	FilePath string
	// Version は helmfile.yaml で宣言された chart のバージョン。
	Version string
	// LatestVersion は chart repository 上の最新バージョン。取得できない場合は "" となる。
	LatestVersion string
	// LatestErr は latest version 取得に失敗した場合のエラー (nil でなければ表示用に警告)。
	LatestErr error

	// releaseInfo は latest 解決に必要な内部情報。ChartList 内部で伝搬するのみで、
	// 呼び出し側からは参照しない。
	releaseInfo manager.ReleaseInfo
}

// ChartList は tazuna.yaml で管理されている helmfile 型 manifest を走査し、
// 各 release で利用している chart をまとめて返します。
//
// checkLatest が true のときのみ、chart repository へ最新バージョンを問い合わせて
// LatestVersion / LatestErr を埋めます。この解決は chart repository へのネットワーク
// アクセスを伴い、大きな HTTP index (prometheus-community 等) を含む構成では
// 数秒〜数十秒かかることがあるため、明示的にオプトインする必要があります。
// 同一の (kind, repoURL, chartName) は 1 度しか問い合わせずキャッシュします。
func (t *TazunaRunner) ChartList(
	ctx context.Context,
	tazuna v1.Tazuna,
	tazunaYAMLPath string,
	checkLatest bool,
) ([]ChartListItem, error) {
	if err := t.expandIncludes(ctx, &tazuna, tazunaYAMLPath); err != nil {
		return nil, errors.WithStack(err)
	}

	baseDir := filepath.Dir(tazunaYAMLPath)
	t.ConvertManifestPathFromCwd(baseDir, &tazuna)

	helmfileMgr := manager.NewHelmfile(t.k8sClient, t.opClient).
		WithEnvironment(t.environment).
		WithRESTConfig(t.restConfig)

	// helmfile 型 manifest を集める。
	type mEntry struct {
		m v1.Manifest
	}
	var targets []mEntry
	for _, m := range tazuna.Spec.Manifests {
		if m.Type == v1.ManifestTypeHelmfile {
			targets = append(targets, mEntry{m: m})
		}
	}

	// helmfile 読み込み・var 解決は並行実行する (1Password 経由の var 解決や
	// helmfile.yaml.gotmpl の render が積み上がると逐次では遅い)。
	perManifest := make([][]manager.ReleaseInfo, len(targets))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(chartListListConcurrency)
	for i, e := range targets {
		g.Go(func() error {
			releases, err := helmfileMgr.ListReleases(gctx, e.m)
			if err != nil {
				t.logger.WarnContext(gctx, "failed to list releases",
					slog.String("manifest", e.m.Name),
					slog.String("path", e.m.Path),
					slog.String("error", err.Error()))
				return errors.Wrapf(err, "failed to list releases for manifest %q", e.m.Name)
			}
			perManifest[i] = releases
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var items []ChartListItem
	for i, e := range targets {
		for _, rel := range perManifest[i] {
			items = append(items, ChartListItem{
				ManifestName: e.m.Name,
				Chart:        rel.Chart,
				Release:      rel.Name,
				FilePath:     rel.HelmfilePath,
				Version:      rel.Version,
				releaseInfo:  rel,
			})
		}
	}

	if !checkLatest || len(items) == 0 {
		return items, nil
	}

	// OCI 参照の latest 解決用に registry client を 1 つ共有する。
	registryClient, err := registry.NewClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create helm registry client")
	}

	// 同じ (kind, repoURL, chartName) の chart は 1 度しか問い合わせない。
	// HTTP index は 100MB を超える repo (prometheus-community 等) もあり、
	// dedup の効果が大きい。逐次実行 (順序付き) で、repository への負荷も抑える。
	type latestResult struct {
		version string
		err     error
	}
	cache := map[string]latestResult{}
	for i := range items {
		key := items[i].releaseInfo.LatestVersionKey()
		if key == "" {
			v, err := helmfileMgr.LatestChartVersion(ctx, items[i].releaseInfo, registryClient)
			items[i].LatestVersion = v
			items[i].LatestErr = err
			continue
		}
		if r, ok := cache[key]; ok {
			items[i].LatestVersion = r.version
			items[i].LatestErr = r.err
			continue
		}
		v, err := helmfileMgr.LatestChartVersion(ctx, items[i].releaseInfo, registryClient)
		cache[key] = latestResult{version: v, err: err}
		items[i].LatestVersion = v
		items[i].LatestErr = err
	}

	return items, nil
}

// chartListListConcurrency は helmfile 読み込み・var 解決の並行数。
// 1Password (op) client を叩く可能性があるため過剰な並列は避ける。
const chartListListConcurrency = 8
