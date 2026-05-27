# リリース

Tazuna のリリースは **タグ push をトリガに GitHub Actions が goreleaser を回す**
という構成です。リリースを切るときに手元から `goreleaser` を直接呼ぶ場面は通常ありません。

ここでは「リリースが切られたときに何が起きるか」「バージョン文字列の供給元」
「成果物の検証手段」をまとめます。

## トリガと出力

- ワークフロー: [`.github/workflows/release.yaml`](https://github.com/pepabo/tazuna/blob/main/.github/workflows/release.yaml)
- 起動条件: `push: tags: ["*"]`（任意のタグ push）
- 公開先: [GitHub Releases](https://github.com/pepabo/tazuna/releases)

タグ名はそのまま `version` 文字列として埋め込まれます。
セマンティックバージョニング（`vX.Y.Z`）に従うことを推奨します（[changelog の自動生成](#changelog) も参照）。

## 出力物

[`.goreleaser.yaml`](https://github.com/pepabo/tazuna/blob/main/.goreleaser.yaml) の
ビルド対象は次のとおりです。

| 軸     | 値                            |
|--------|-------------------------------|
| GOOS   | `linux` / `darwin`            |
| GOARCH | `amd64` / `arm64`             |
| CGO    | `CGO_ENABLED=0`               |
| ldflags| `-s -w -trimpath` + バージョン埋め込み |

成果物のアーカイブ名は `tazuna_<Os>_<Arch>.tar.gz`（`amd64` は `x86_64` に正規化）で、
リリースには次のものが添付されます。

- 各 OS/arch の `.tar.gz`（バイナリ）
- `checksums.txt`（SHA256）
- 各アーカイブ + ソースの SBOM (`*.sbom.json`、SPDX)
- `checksums.txt.sigstore.json`（cosign のキーレス署名バンドル）

GitHub Actions の [`actions/attest-build-provenance`](https://github.com/actions/attest-build-provenance)
が SLSA build provenance を別途生成し、`gh attestation verify` で検証できる形に
紐付けます。

## バージョン文字列の埋め込み

`main.go` には次の var が宣言されていて、リリースビルド時に `-X` で値が注入されます。

```go
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)
```

`.goreleaser.yaml` の `ldflags` で:

```text
-X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}
```

注入された値は `cmd.SetVersionInfo` 経由で [`tazuna version`](../reference/cli/version.md)
に表示されます。

`go install` / `make build` などローカルビルドの場合は注入されないため、
`version` / `commit` / `date` はそのまま `dev` / `none` / `unknown` になります。

## changelog

`.goreleaser.yaml` の `changelog` セクションが GitHub Releases の説明文を組み立てます。

- 並び: コミット昇順 (`sort: asc`)
- 除外: `^docs:` / `^test:` プレフィックスの commit
- フォーマット: ヘッダ `## Tazuna {{.Version}}` と、フッタに前リリースとの compare リンク

PR のタイトル / commit メッセージ規約は CONTRIBUTING.md / PR テンプレートが基準です。
**`docs:` / `test:` プレフィックスはリリースノートに出ない** ので、プロダクトの動作変更
ではない PR にはこれらを付けると棚卸しがしやすくなります。

## 成果物の検証

リリースを使う側の検証手段はおおむね 3 つあります。

```bash
# 1. checksum の検証
sha256sum -c checksums.txt

# 2. checksums.txt の署名検証（cosign keyless）
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  checksums.txt

# 3. SLSA provenance の検証
gh attestation verify <file> --repo pepabo/tazuna
```

3 つ目は GitHub CLI 経由で SLSA build provenance を確認する手段です。
リリース直後でも内部 CI でも使えます。

## 切るときの実務

1. `main` が安定していることを確認する（CI が green）。
2. `vX.Y.Z` 形式のタグを切って push する。
3. `Release` ワークフローが走り、Releases が公開されることを確認する。
4. 必要なら自動生成された changelog を手で整える。

ドキュメントサイトの公開は別ワークフロー（[`docs.yaml`](https://github.com/pepabo/tazuna/blob/main/.github/workflows/docs.yaml)）
で、こちらは `main` への push をトリガに動きます。リリース切りとは独立しています。
