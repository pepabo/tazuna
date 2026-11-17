// Package tmpl は tazuna.yaml (および include ファイル) を Go template として
// 解釈するための薄いラッパーを提供します。tazuna.yaml は Unmarshal される前に
// 一度 Go template として描画されるため、`{{ .Environment }}` のような変数を
// 埋め込んで環境ごとに異なる値を注入できます。
package tmpl

import (
	"bytes"
	"text/template"

	"github.com/cockroachdb/errors"
)

// Data は tazuna.yaml を Go template として描画する際に参照できる変数の集合です。
// フィールドを追加した場合は docs/src/reference/tazuna-yaml.md の
// 「テンプレート変数」表も更新してください。
type Data struct {
	// Environment は -e/--environment フラグで渡された環境名です。
	// フラグが渡されていない場合は空文字列になります。
	Environment string
}

// Render は raw を Go template として解釈し、data を適用した結果を返します。
// name はエラーメッセージに使う識別子 (通常はファイルパス) です。
//
// missingkey=error を指定しているため、map に対して未定義キーを参照するテンプレートは
// エラーになります。Data のような struct に存在しないフィールド (例: `{{ .Unknown }}`)
// を参照した場合も text/template が実行時エラーを返します。
//
// tfstate / tfstateJSON は stub (空値を返し backend に触れない) として登録されます。
// 実際の値で描画したい場合は RenderWithFuncs に tfstate.Resolver.TemplateFuncs を渡してください。
func Render(name string, raw []byte, data Data) ([]byte, error) {
	return RenderWithFuncs(name, raw, data, nil)
}

// StubFuncs は tazuna が提供する template 関数の stub を返します。stub は
// backend にアクセスせず空値を返すため、「spec.tfstates を読み取るための
// 1 回目の描画」や「テンプレート式を含むかどうかの判定」に使えます。
//   - tfstate: 空文字列
//   - tfstateJSON: "null"
func StubFuncs() template.FuncMap {
	return template.FuncMap{
		"tfstate":     func(_, _ string) string { return "" },
		"tfstateJSON": func(_, _ string) string { return "null" },
	}
}

// RenderWithFuncs は Render と同じですが、funcs で template 関数を上書き / 追加できます。
// funcs に含まれない tazuna 提供関数は StubFuncs の stub になります。
func RenderWithFuncs(name string, raw []byte, data Data, funcs template.FuncMap) ([]byte, error) {
	fm := StubFuncs()
	for k, f := range funcs {
		fm[k] = f
	}
	t, err := template.New(name).Option("missingkey=error").Funcs(fm).Parse(string(raw))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s as a Go template", name)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, errors.Wrapf(err, "failed to render %s as a Go template", name)
	}

	return buf.Bytes(), nil
}
