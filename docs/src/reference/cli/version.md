# `tazuna version`

バイナリに埋め込まれたバージョン情報を出力します。

```text
tazuna version
tazuna --version
```

両者は等価です。

## 振る舞い

次の形式で 1 行出力して終了します。

```text
tazuna <version> (commit <commit>, built <date>, <os>/<arch>)
```

- `<version>` — リリース時のタグ。ローカルビルドでは `dev`。
- `<commit>` — リリース時の commit hash。未注入時は `none`。
- `<date>` — リリース時のビルド日時。未注入時は `unknown`。
- `<os>/<arch>` — `runtime.GOOS` / `runtime.GOARCH`。

`<version>` / `<commit>` / `<date>` は goreleaser によるリリース時に注入されます。
`go install` / `go run` などローカルビルドでは未注入のためデフォルト値が出ます。

## フラグ

固有フラグはありません。引数も受け付けません。

## 例

```bash
tazuna version
tazuna --version
```
