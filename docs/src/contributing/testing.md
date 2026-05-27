# テスト

Tazuna のテストは **unit / integration / e2e** の 3 レイヤです。
各レイヤは `make` ターゲットと 1 対 1 で対応しており、PR で常に動くのは unit
（CI）、e2e は KinD を前提にした手動レイヤです。

## レイヤと走らせ方

| レイヤ        | コマンド                  | 対象                | 前提 |
|---------------|---------------------------|---------------------|------|
| unit          | `make test`               | `go test ./...`     | なし。GitHub Actions の `CI` ワークフローで毎 push / PR 時に走る。 |
| integration   | `make test-integration`   | `go test -tags=integration ./...` | なし。build tag `integration` を付けた追加テストが対象。 |
| e2e           | `make test-e2e`           | `go test -tags=e2e -count=1 ./test/e2e/...` | KinD クラスタ。内部で `make build && make devenv-create` が走る。 |
| すべて        | `make test-all`           | unit → integration → e2e |  e2e と同じ |
| カバレッジ    | `make cover`              | `-race -covermode=atomic -coverprofile=coverage.out` で unit を実行、サマリ出力 | なし |

CI（`.github/workflows/ci.yaml`）では `go test -race -covermode=atomic -coverprofile=coverage.out ./...`
が走るため、内容は `make cover` とおおむね一致します。

## unit テスト

すべてのパッケージで `*_test.go` として書かれています。Go の標準的なテストです。
KinD / 外部 CLI に依存せず、`go test ./...` のみで完結します。

PR レビュー対象になるのはこのレイヤが green であることが大前提です。

## integration テスト

build tag `integration` を付けた追加テストです。`make test-integration` で実行します。
外部依存はないが、unit テストには重すぎるシナリオを切り分ける場所として使われます。

`go test -tags=integration ./...` を直接叩いても同じです。

## e2e テスト

`test/e2e/` に置かれた本物のクラスタを使ったテストです。
build tag `e2e` で隔離され、`./test/e2e/...` だけが対象になります。

`make test-e2e` を実行すると、内部で次が順に走ります。

1. `make build`（`./tazuna` を作る）
2. `make devenv-create`（KinD クラスタ `tazuna` を立てる、既存なら context を切り替え）
3. `go test -tags=e2e -count=1 ./test/e2e/...`

`-count=1` が付くのは、e2e テストのキャッシュを禁止して毎回実走させるためです。
`make devenv-destroy` で KinD クラスタは片付きます（`kind delete cluster --name tazuna`）。

### KinD クラスタの仕様

| 項目 | 値 |
|------|----|
| クラスタ名 | `tazuna` |
| kubeconfig context | `kind-tazuna` |
| 設定ファイル | [`.github/kind-config.yaml`](https://github.com/pepabo/tazuna/blob/main/.github/kind-config.yaml) |

CI 用と開発者ローカル用で同じ KinD 設定を共有しているため、ローカルで通る e2e は
基本的に CI でも通ります。

### テストデータ

e2e の fixture は `test/e2e/testdata/` 配下にケースごとのディレクトリで置かれています。
`tazuna.yaml` と、`type` に対応した実体（`kustomize/` / `helmfile/` 等）が同居する作りです。

```text
test/e2e/testdata/
├── kustomize-minimal/         # type: kustomize の最小ケース
├── helmfile-minimal/          # type: helmfile の最小ケース
├── parallel-minimal/          # type: parallel
├── testplugin-minimal/        # Test plugin (WaitUntil/ExistNonExist) の基本
├── testplugin-cel/            # WaitUntil の CEL 式が複雑なケース
├── tags-filter-minimal/       # --tags フィルタ
├── state-minimal/             # state list/diff/sync
├── state-modified/            # state diff の "modified" 判定
├── destroy-minimal/           # tazuna destroy
└── check-invalid/             # tazuna check の異常系
```

新しい機能を入れるときは、対応する fixture ディレクトリを追加し、最小構成の
`tazuna.yaml` を 1 つ書き、`test/e2e/` 配下に `*_test.go` を追加する流れが一般的です。

## lint

```bash
make lint
```

`golangci-lint run` を呼ぶだけです。`golangci-lint` のバージョンは `mise.toml` で
管理しているため、`mise install` で揃えれば追加の手当ては不要です。

CI でも同じ `golangci-lint` が走ります。PR を出す前にローカルで通しておくと往復が減ります。
