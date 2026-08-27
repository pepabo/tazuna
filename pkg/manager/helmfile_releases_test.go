package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// TestHelmfile_ListReleases は helmfile.yaml から宣言された release 情報 (name,
// namespace, chart, version, helmfile 実パス) を正しく取り出せることを検証する。
// chart の実 render / repository へのアクセスは行わないため、外部依存なしで
// 実行できる。
func TestHelmfile_ListReleases(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	helmfilePath := filepath.Join(dir, "helmfile.yaml")
	body := `repositories:
  - name: argo-cd
    url: https://argoproj.github.io/argo-helm
releases:
  - name: argo-cd
    namespace: argocd
    chart: argo-cd/argo-cd
    version: 9.0.5
  - name: karpenter-crd
    namespace: karpenter
    chart: oci://public.ecr.aws/karpenter/karpenter-crd
    version: 1.5.6
`
	if err := os.WriteFile(helmfilePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHelmfile(nil, nil)
	m := v1.Manifest{
		Name:     "test",
		Type:     v1.ManifestTypeHelmfile,
		Path:     dir,
		Helmfile: v1.DefaultHelmfile(),
	}
	got, err := h.ListReleases(context.Background(), m)
	if err != nil {
		t.Fatalf("ListReleases returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(got))
	}

	want := []struct {
		Name, Namespace, Chart, Version, kind string
	}{
		{"argo-cd", "argocd", "argo-cd/argo-cd", "9.0.5", "http"},
		{"karpenter-crd", "karpenter", "oci://public.ecr.aws/karpenter/karpenter-crd", "1.5.6", "oci"},
	}
	for i, w := range want {
		g := got[i]
		if g.Name != w.Name || g.Namespace != w.Namespace || g.Chart != w.Chart || g.Version != w.Version {
			t.Errorf("release[%d]: got %+v, want %+v", i, g, w)
		}
		if g.HelmfilePath != helmfilePath {
			t.Errorf("release[%d].HelmfilePath: got %s, want %s", i, g.HelmfilePath, helmfilePath)
		}
		if g.chartSource.kind != w.kind {
			t.Errorf("release[%d].chartSource.kind: got %s, want %s", i, g.chartSource.kind, w.kind)
		}
	}
}

// TestResolveChartSource は chart 参照文字列を kind とメタデータへ分解するロジックを
// 直接検証する。ネットワークアクセスは発生しない。
func TestResolveChartSource(t *testing.T) {
	t.Parallel()

	repos := map[string]helmfileRepository{
		"argo-cd": {Name: "argo-cd", URL: "https://argoproj.github.io/argo-helm"},
		"myoci":   {Name: "myoci", URL: "oci://ghcr.io/example"},
		"ociflag": {Name: "ociflag", URL: "ghcr.io/example", OCI: true},
	}

	tests := []struct {
		name      string
		chartRef  string
		wantKind  string
		wantChart string
		wantURL   string
	}{
		{"http-alias", "argo-cd/argo-cd", "http", "argo-cd", "https://argoproj.github.io/argo-helm"},
		{"oci-alias-url", "myoci/mychart", "oci", "mychart", "oci://ghcr.io/example"},
		{"oci-alias-flag", "ociflag/mychart", "oci", "mychart", "oci://ghcr.io/example"},
		{"oci-direct", "oci://public.ecr.aws/karpenter/karpenter-crd", "oci", "karpenter-crd", "oci://public.ecr.aws/karpenter"},
		{"local-path", "./charts/mychart", "local", "", ""},
		{"unknown-alias", "unknown/mychart", "local", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := resolveChartSource("", tc.chartRef, repos)
			if src.kind != tc.wantKind {
				t.Errorf("kind: got %s, want %s", src.kind, tc.wantKind)
			}
			if src.chartName != tc.wantChart {
				t.Errorf("chartName: got %s, want %s", src.chartName, tc.wantChart)
			}
			if src.repoURL != tc.wantURL {
				t.Errorf("repoURL: got %s, want %s", src.repoURL, tc.wantURL)
			}
		})
	}
}
