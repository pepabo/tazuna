package manager

import (
	"strings"
	"testing"
	"text/template"
)

// renderWithFuncs は helmfileTemplateFuncs を使って文字列テンプレートを render するヘルパ。
func renderWithFuncs(t *testing.T, tmpl string, data any) string {
	t.Helper()
	tp, err := template.New("t").Funcs(helmfileTemplateFuncs()).Parse(tmpl)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	var b strings.Builder
	if err := tp.Execute(&b, data); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	return b.String()
}

// tazunahub v2.10.0 の argo-workflows helmfile.yaml.gotmpl が使う
// `{{ .StateValues.x | toYaml | indent N }}` 形式が render できることを保証する。
func TestHelmfileTemplateFuncs_ToYaml(t *testing.T) {
	data := map[string]any{
		"StateValues": map[string]any{
			"list": []any{"argo", "batch"},
			"m":    map[string]any{"key": "value"},
		},
	}

	got := renderWithFuncs(t, `{{ .StateValues.list | toYaml }}`, data)
	if !strings.Contains(got, "- argo") || !strings.Contains(got, "- batch") {
		t.Errorf("toYaml(list) = %q; want YAML sequence", got)
	}
	// 末尾改行が除去されていること (helm と同じ挙動 = indent と組み合わせても崩れない)
	if strings.HasSuffix(got, "\n") {
		t.Errorf("toYaml should trim trailing newline, got %q", got)
	}

	gotMap := renderWithFuncs(t, `{{ .StateValues.m | toYaml }}`, data)
	if !strings.Contains(gotMap, "key: value") {
		t.Errorf("toYaml(map) = %q; want 'key: value'", gotMap)
	}

	// indent と組み合わせた実利用形
	indented := renderWithFuncs(t, `x:
{{ .StateValues.list | toYaml | indent 2 }}`, data)
	if !strings.Contains(indented, "  - argo") {
		t.Errorf("toYaml|indent 2 = %q; want two-space indented sequence", indented)
	}
}

func TestHelmfileTemplateFuncs_FromYaml(t *testing.T) {
	got := renderWithFuncs(t, `{{ (fromYaml "key: value").key }}`, nil)
	if got != "value" {
		t.Errorf("fromYaml round trip = %q; want 'value'", got)
	}
}
