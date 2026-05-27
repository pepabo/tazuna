# 全体アーキテクチャ

このページでは Tazuna を構成する主なコンポーネントと、
それらが `tazuna apply` のときにどう協調するかを俯瞰します。

ここでは責務の境界だけを扱います。具体的なディレクトリ構成や
Go パッケージの分割方針は意図的に触れません。

## レイヤ図

おおまかには次の 3 層構造です。

```
+--------------------------------------------------+
|  CLI                                             |
|   apply / build / check / destroy / state ...    |
+--------------------------------------------------+
                         |
                         v
+--------------------------------------------------+
|  Runner                                          |
|   - load tazuna.yaml / expand includes           |
|   - verify context_matches / filter by tags      |
|   - dispatch each manifest to a Manager          |
+--------------------------------------------------+
                         |
           +-------------+-------------+
           v                           v
+---------------------+     +---------------------+
|  Manager            |     |  Test plugin        |
|                     |     |   wait-until /      |
|  kustomize /        |     |   exist-nonexist    |
|  helmfile /         |     +---------------------+
|  oras /             |
|  genesissecret /    |
|  parallel           |
+---------------------+
           |
           v
+---------------------+
|  Kubernetes cluster |
+---------------------+
```

各 manifest は **1 つの Manager** を介してクラスタに反映されます。
適用前後の検証は **Test plugin** に切り出されており、
manifest 単位でも tazuna.yaml 全体でも実行できます。

## 主要コンポーネント

Tazuna は内部的に次のコンポーネントが組み合わさって動きます。
ここでは各コンポーネントが「何を入力に取り、何を出力するか」という責務だけを示します。

### CLI

サブコマンド（`apply`, `build`, `check`, `destroy`, `state list`, `state diff`,
`state sync`, `secret-to-genesissecret`, `version` など）の入り口です。
フラグの解釈と Runner の組み立てだけを担当し、ロジック自体は持ちません。

### Runner

Tazuna 全体のオーケストレータです。次を担当します。

- `tazuna.yaml` のロード
- `includes` の展開
- `tazuna.yaml` 起点の相対パスを実行時パスへの変換
- `--tags` によるフィルタ
- 各 manifest を対応する Manager へ渡す
- 適用後の Test plugin の実行

Runner は manifest type の種類を知りません。
「どの type に対してどの Manager を使うか」というマップだけを持ち、
それ以外は Manager 側に委ねます。

### Manager

manifest type ごとの「実際の適用方法」を実装するコンポーネントです。
すべての Manager は次の 3 つの操作を提供します。

- **Apply** — その manifest をクラスタへ反映する
- **Destroy** — その manifest が入れたものをクラスタから取り除く
- **Build** — クラスタには触らず、適用したら何が入るかだけを生成する

Runner はこの 3 操作だけを介して manifest type を均一に扱います。
個別のバックエンドの責務分担は [リファレンス](../reference/index.md) を参照してください。

### Validator

`tazuna.yaml` のロード直後に走るスキーマ・パス整合性検証です。
`apply` も `check` も最初にここを通り、不正な YAML や
存在しない `path` を **クラスタに触る前に** 弾きます。

### Context guard

`tazuna.yaml` の `context_matches` を読み、現在の kubeconfig context 名が
そのパターン（複数の正規表現）に合致しているかを検証します。
合致しなければ apply / destroy はここで終了し、クラスタには一切触れません。

### State store

Tazuna が「自分が入れたリソースの指紋」をクラスタに記録するための仕組みです。
保存先はクラスタ内の ConfigMap で、`state list` / `state diff` / `state sync` は
ここを起点に動きます。

### Secret provider

`GenesisSecret` および helmfile の `vars.op` から参照される、
「シークレットの取得元」を抽象化したコンポーネントです。
現状は 1Password 向けの実装が組み込まれています。

### Test plugin

manifest 適用後（または `tazuna.yaml` 全体の適用後）に走らせる検証の仕組みです。
組み込みで次の 2 種が利用できます。

- `wait-until` — 指定リソースが存在 / Ready / Available になるまで待つ
- `exist-nonexist` — 指定リソースが存在 / 非在 であるべきことを表明する

### Hint

helmfile の `vars` が取りうる値の型・フォーマット
（hostname / URL / email / IP / CIDR / UUID / semver / datetime など）を
宣言的に検証するための補助コンポーネントです。

### Prompt

破壊的操作の Yes/No 確認といった対話 I/O を抽象化するコンポーネントです。
非対話モードやテスト時に振る舞いを差し替えるためのものです。

## `tazuna apply` のときの流れ

ここでは「どのコンポーネントが何をするか」の観点で並べておきます。

1. **CLI** がフラグを解釈し、Runner を組み立てる。
2. **Validator** が `tazuna.yaml` を読み、スキーマと `path` の存在を検証する。
3. `spec.context_matches` があれば **Context guard** が kubeconfig を検証する。
4. **Runner** が `includes` を展開し、`manifests[].path` を実行時パスへ変換する。
5. `manifests` を順に走査し、`--tags` で除外されないものを
   対応する **Manager** に渡す。
6. 各 Manager は内部で kustomize / helmfile / oras pull / 1Password 取得 などを呼び出し、
   結果をクラスタへ反映する。State store に指紋が書き込まれるのもここ。
7. manifest 単位 / 全体の Tests があれば **Test plugin** が走る。

`tazuna state sync` はこの 1〜4 までを使ったうえで、
各 Manager の `Build` 結果から **State diff** を組み、追加分・変更分だけを apply します。
