package manager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitRepoAlias(t *testing.T) {
	tests := []struct {
		name      string
		chartRef  string
		wantAlias string
		wantChart string
		wantOK    bool
	}{
		{
			name:      "repo alias form",
			chartRef:  "argo-cd/argo-cd",
			wantAlias: "argo-cd",
			wantChart: "argo-cd",
			wantOK:    true,
		},
		{
			name:      "different chart name",
			chartRef:  "prometheus-community/kube-prometheus-stack",
			wantAlias: "prometheus-community",
			wantChart: "kube-prometheus-stack",
			wantOK:    true,
		},
		{
			name:     "oci scheme is not alias form",
			chartRef: "oci://public.ecr.aws/karpenter/karpenter-crd",
			wantOK:   false,
		},
		{
			name:     "explicit relative local path (dot)",
			chartRef: "./charts/foo",
			wantOK:   false,
		},
		{
			name:     "explicit parent local path (dot dot)",
			chartRef: "../charts/foo",
			wantOK:   false,
		},
		{
			name:     "absolute path",
			chartRef: "/abs/charts/foo",
			wantOK:   false,
		},
		{
			name:     "single segment (bare local chart)",
			chartRef: "mychart",
			wantOK:   false,
		},
		{
			name:     "deep local path is not alias form",
			chartRef: "charts/sub/foo",
			wantOK:   false,
		},
		{
			name:     "trailing slash",
			chartRef: "argo-cd/",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alias, chart, ok := splitRepoAlias(tt.chartRef)
			if ok != tt.wantOK {
				t.Fatalf("splitRepoAlias(%q) ok = %v, want %v", tt.chartRef, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if alias != tt.wantAlias || chart != tt.wantChart {
				t.Fatalf("splitRepoAlias(%q) = (%q, %q), want (%q, %q)",
					tt.chartRef, alias, chart, tt.wantAlias, tt.wantChart)
			}
		})
	}
}

func TestRepositoryIsOCI(t *testing.T) {
	tests := []struct {
		name string
		repo helmfileRepository
		want bool
	}{
		{
			name: "http url",
			repo: helmfileRepository{URL: "https://argoproj.github.io/argo-helm"},
			want: false,
		},
		{
			name: "oci url",
			repo: helmfileRepository{URL: "oci://public.ecr.aws/karpenter"},
			want: true,
		},
		{
			name: "oci flag with http-like url",
			repo: helmfileRepository{URL: "registry.example.com/charts", OCI: true},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repositoryIsOCI(tt.repo); got != tt.want {
				t.Fatalf("repositoryIsOCI(%+v) = %v, want %v", tt.repo, got, tt.want)
			}
		})
	}
}

func TestOCIChartRef(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		chart   string
		want    string
	}{
		{
			name:    "oci url without trailing slash",
			repoURL: "oci://public.ecr.aws/karpenter",
			chart:   "karpenter-crd",
			want:    "oci://public.ecr.aws/karpenter/karpenter-crd",
		},
		{
			name:    "oci url with trailing slash",
			repoURL: "oci://public.ecr.aws/karpenter/",
			chart:   "karpenter-crd",
			want:    "oci://public.ecr.aws/karpenter/karpenter-crd",
		},
		{
			name:    "url without oci scheme (oci: true)",
			repoURL: "registry.example.com/charts",
			chart:   "mychart",
			want:    "oci://registry.example.com/charts/mychart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ociChartRef(tt.repoURL, tt.chart); got != tt.want {
				t.Fatalf("ociChartRef(%q, %q) = %q, want %q", tt.repoURL, tt.chart, got, tt.want)
			}
		})
	}
}

func TestReleaseNeedsOCI(t *testing.T) {
	repos := map[string]helmfileRepository{
		"argo-cd":   {Name: "argo-cd", URL: "https://argoproj.github.io/argo-helm"},
		"karpenter": {Name: "karpenter", URL: "oci://public.ecr.aws/karpenter"},
	}

	tests := []struct {
		name string
		rel  helmfileRelease
		want bool
	}{
		{
			name: "direct oci chart",
			rel:  helmfileRelease{Chart: "oci://public.ecr.aws/karpenter/karpenter-crd"},
			want: true,
		},
		{
			name: "http repo alias",
			rel:  helmfileRelease{Chart: "argo-cd/argo-cd"},
			want: false,
		},
		{
			name: "oci repo alias",
			rel:  helmfileRelease{Chart: "karpenter/karpenter-crd"},
			want: true,
		},
		{
			name: "undeclared alias (treated as local path)",
			rel:  helmfileRelease{Chart: "unknown/chart"},
			want: false,
		},
		{
			name: "bare local chart",
			rel:  helmfileRelease{Chart: "mychart"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := releaseNeedsOCI(&tt.rel, repos); got != tt.want {
				t.Fatalf("releaseNeedsOCI(%q) = %v, want %v", tt.rel.Chart, got, tt.want)
			}
		})
	}
}

func TestLoadValueFile_GotmplRendered(t *testing.T) {
	dir := t.TempDir()

	// helmfile 実運用と同様、先頭にコメント + 空行を挟んでから go template action を書く。
	// このパターンは素の YAML parse だと '{' から始まる flow mapping と誤認され失敗する
	// (README で扱っている kube-prometheus-stack の再現ケース)。
	content := `# comment header
# another comment line

{{- $thanosEnabled := eq .StateValues.env "staging" }}
fullnameOverride: kube-prometheus-stack
env: {{ .StateValues.env | quote }}
thanosEnabled: {{ $thanosEnabled }}
`
	path := filepath.Join(dir, "values.yaml.gotmpl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	vars := map[string]any{"env": "staging"}
	got, err := loadValueFile(dir, "values.yaml.gotmpl", vars, "default")
	if err != nil {
		t.Fatalf("loadValueFile: %v", err)
	}

	want := map[string]any{
		"fullnameOverride": "kube-prometheus-stack",
		"env":              "staging",
		"thanosEnabled":    true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadValueFile mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestLoadValueFile_PlainYamlNotRendered(t *testing.T) {
	dir := t.TempDir()

	// 非 .gotmpl の values は go template として解釈してはならない。
	// {{ }} を含むリテラル文字列 (例: helm chart 側で最終解釈する式) をそのまま保持する。
	content := `image:
  tag: "{{ .Chart.AppVersion }}"
`
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := loadValueFile(dir, "values.yaml", map[string]any{"env": "staging"}, "default")
	if err != nil {
		t.Fatalf("loadValueFile: %v", err)
	}

	image, ok := got["image"].(map[string]any)
	if !ok {
		t.Fatalf("image key missing or wrong type: %#v", got["image"])
	}
	if image["tag"] != "{{ .Chart.AppVersion }}" {
		t.Fatalf("plain yaml was templated: got tag=%v", image["tag"])
	}
}
