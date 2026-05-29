# はじめかた

このセクションは、Tazuna を初めて手元で動かせる状態にするまでを通します。
ここを通したあと、実際の `tazuna.yaml` の書きかたは
[ガイド - 最初の tazuna.yaml を書く](../guides/first-tazuna-yaml.md) に進んでください。

ゴール:

- 自分のマシンに `tazuna` バイナリが入っている
- `tazuna version` が動く
- どんなクラスタ・どんな外部ツールが前提なのか把握できている

## 1. インストール

`tazuna` は単一バイナリです。以下のいずれかの方法で入れます。

### リリースバイナリを使う

GitHub の [Releases](https://github.com/pepabo/tazuna/releases) から、
お使いの OS / arch のアーカイブをダウンロードし、`tazuna` を `PATH` に置きます。

```bash
# 例（macOS arm64 / Linux amd64 などのアーカイブを展開後）
mv tazuna /usr/local/bin/
tazuna version
```

CI で固定バージョンを使いたい場合や、再現性を担保したい場合はこの方法を推奨します。

### `go install` を使う

Go 1.x の toolchain がある場合は、ソースから直接ビルドできます。

```bash
go install github.com/pepabo/tazuna@latest
```

`@latest` の代わりに `@v0.x.y` でバージョンを固定できます。
バイナリは `$(go env GOBIN)`（未設定なら `$(go env GOPATH)/bin`）に入ります。

### ソースから直接ビルドする

リポジトリを `clone` した状態から自分でビルドする場合は次のとおりです。

```bash
make build      # ./tazuna が生成される
make install    # /usr/local/bin にインストール（sudo を使う）
```

## 2. 動作確認

インストール後、何も設定せずに次のコマンドが通ることを確認します。

```bash
tazuna version
tazuna --help
```

`tazuna version` の出力は次のような形式です（実際の値は環境によって変わります）。

```text
tazuna v0.1.0 (commit abc1234, built 2026-05-25T03:21:00Z, darwin/arm64)
```

ローカルビルドの場合は `version` / `commit` / `built` がそれぞれ `dev` / `none` /
`unknown` になります（[`tazuna version`](../reference/cli/version.md) を参照）。

## 3. 前提を揃える

`tazuna` がクラスタに対して動作するためには、いくつかの前提が必要です。
すべて必須ではありません。下の表の「必須」だけまず揃えればよく、
1Password / git は使う機能に応じて任意で揃えます。

| 依存                 | 必須？                        | 説明 |
|----------------------|-------------------------------|------|
| Kubernetes クラスタ  | ◯（`apply` / `destroy` / `state ...` を使うとき） | コントロールプレーンの用意は Tazuna の範囲外です。手元で試すなら [KinD](https://kind.sigs.k8s.io/) / [minikube](https://minikube.sigs.k8s.io/)、リモートなら EKS / GKE / AKS / `kubeadm` などで構いません。 |
| `kubeconfig` の current-context | ◯（同上） | `kubectl config current-context` で、入れたい先のクラスタを指していることを確認します。`kubectl get nodes` が通る状態にしておきます。 |
| `kubectl`            | -（推奨）                     | Tazuna 自身は使いませんが、`kubeconfig` 操作や反映結果の確認で使います。 |
| `kustomize` / `helmfile` / `helm` バイナリ | -（不要） | Tazuna は Go ライブラリとして組み込んでいるため、外部バイナリのインストールは不要です。 |
| 1Password CLI (`op`) | -（[`type: genesissecret`](../reference/manifest-types/genesissecret.md) や [helmfile.vars の `from: op`](../reference/manifest-types/helmfile.md#vars) を使うときのみ） | `op` コマンドが `PATH` にあり、サービスアカウントで認証されていることが前提です。 |
| `git`                | -（任意） | `tazuna apply` 時に State の `_metadata.gitCommitHash` を埋めるために使います。リポジトリ外で実行した場合や `git` が無い場合は、エラーにせず空文字で記録されます。 |

クラスタの選び方や `kubeconfig` の扱いについては、ガイドの
[前提](../guides/first-tazuna-yaml.md#前提) でも触れています。

## 4. 次に進む

ここまでで `tazuna` が手元で動く状態になりました。

- [ガイド - 最初の tazuna.yaml を書く](../guides/first-tazuna-yaml.md) で、
  実際に `tazuna.yaml` を 1 枚書いて `apply` するところまで進めます。
- 仕様だけを引きたいときは [リファレンス](../reference/index.md) を参照してください。
- 「なぜそうなっているか」が知りたい場合は [概念](../concepts/index.md) を参照してください。
