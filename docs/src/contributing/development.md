# 開発環境

このページは、Tazuna 本体に手を入れたい人がローカル環境を整え、
コード変更をビルド・実行・確認するまでの手順をまとめます。
ドキュメントやリリースについては別ページ（[ドキュメント](./documentation.md) /
[リリース](./releases.md)）を参照してください。

## toolchain は mise で揃える

リポジトリには [`mise.toml`](https://github.com/pepabo/tazuna/blob/main/mise.toml) が
入っており、必要な toolchain がピン留めされています。

```toml
[tools]
go = "1.26.0"
golangci-lint = "latest"
helm = "latest"
```

`mise` をインストール済みであれば、リポジトリのルートで `mise install` を実行すれば
これらが揃います。Go や golangci-lint をシステム側で別管理している場合は、`mise.toml`
と一致するバージョンを自分で揃えてください。

なお `go.mod` が要求する Go のバージョン (`go 1.26.x`) が `mise.toml` のピン留めより
新しい場合は、ビルド時に Go の toolchain ダウンロード機構が差分を吸収します。意図的に
toolchain ダウンロードを避けたい場合は、`mise.toml` を `go.mod` の `go` 行に揃えて使ってください。

`helm` バイナリは Tazuna 自身は使いません（[Helmfile backend](../reference/manifest-types/helmfile.md)
は Go ライブラリとして組み込んでいます）。`mise.toml` に入っているのは、`helm` を
依存ツールとして扱う開発フローを将来サポートする余地を残すためのものです。

## 主要な `make` ターゲット

`Makefile` の中身に対応するターゲットだけ列挙します。

| ターゲット             | 中身 |
|------------------------|------|
| `make build`           | `go build .` で `./tazuna` を生成 |
| `make install`         | `make build` の後 `sudo mv tazuna /usr/local/bin` |
| `make format`          | `go fmt ./...` |
| `make lint`            | `golangci-lint run` |
| `make test`            | `go test ./...`（unit テスト） |
| `make test-integration` | `go test -tags=integration ./...` |
| `make test-e2e`        | `make build && make devenv-create` の後 `go test -tags=e2e -count=1 ./test/e2e/...` |
| `make test-all`        | unit + integration + e2e |
| `make cover`           | `-race -covermode=atomic -coverprofile=coverage.out` でテスト後、サマリ出力 |
| `make all`             | `format → test → build → lint` を順に実行 |
| `make devenv-create`   | `kind` で `tazuna` という名前のクラスタを立てる（既存なら context を切り替え） |
| `make devenv-destroy`  | `kind delete cluster --name tazuna` |

KinD クラスタ名は **固定で `tazuna`**、kubeconfig context 名は `kind-tazuna` です。
e2e は KinD クラスタを前提にしているため、`make test-e2e` を初めて走らせるときは
内部で `make devenv-create` が動きます（[テスト](./testing.md) も参照）。

## リポジトリ構成

主要なディレクトリの責務はおおむね次のとおりです。

| パス                | 役割 |
|---------------------|------|
| `main.go`           | エントリポイント。`cmd.Execute()` を呼ぶだけ。 |
| `cmd/`              | Cobra のサブコマンド定義（`apply` / `build` / `check` / `destroy` / `state ...` / `secret-to-genesissecret` / `tags` / `version`）。 |
| `cmd/internal/`     | サブコマンド間で共有する内部ユーティリティ。 |
| `api/v1/`           | YAML スキーマに対応する Go 構造体定義（`tazuna.yaml` / `tazuna.hint.yaml` / GenesisSecret / TestPluginSpec / ORAS）。 |
| `pkg/runner/`       | `tazuna apply` 全体のオーケストレーション。 |
| `pkg/manager/`      | Manifest type 別の Manager 実装（`kustomize` / `helmfile` / `genesis_secret` / `parallel`、および `oras/` サブパッケージ）。 |
| `pkg/state/`        | State の表現と ConfigMap 永続化。 |
| `pkg/testplugin/`   | `WaitUntil` / `ExistNonExist` 実装。 |
| `pkg/genesissecret/` | Provider インターフェースと 1Password 向け実装。 |
| `pkg/hint/`         | `tazuna.hint.yaml` のロードと検証。 |
| `pkg/op/`           | `op`（1Password CLI）の呼び出し。 |
| `pkg/validator/`    | `tazuna.yaml` のバリデーション。 |
| `pkg/context/`      | `context_matches` の評価。 |
| `pkg/prompt/`       | 対話入力の抽象（destroy 時の Yes/No など）。 |
| `pkg/resource/`     | 反映時に共通で使う Kubernetes リソース操作ヘルパ。 |
| `test/e2e/`         | E2E テスト本体と fixture (`testdata/`)。 |
| `docs/`             | このドキュメントサイト。 |

リファレンスでよく出てくる Manager / Runner / Validator などの責務分担は
[全体アーキテクチャ](../concepts/architecture.md) を参照すると引き合わせやすいです。

## ローカルバイナリで挙動を試す

`make build` で生成した `./tazuna` を直接呼べば、リリース版のかわりに開発中の
バイナリで挙動を試せます。

```bash
make build
./tazuna check -f path/to/tazuna.yaml
./tazuna build -f path/to/tazuna.yaml --tags infra
```

`PATH` に置きたいときは `make install` を使います（`sudo` が必要です）。
KinD で実機検証する場合は `make devenv-create` でクラスタを立て、
`kubectl config use-context kind-tazuna` で current-context を切り替えてから動かします。
