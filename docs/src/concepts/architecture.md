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
|   apply / build / check / destroy / plan /       |
|   status / state list/diff/drift / ...           |
+--------------------------------------------------+
                         |
                         v
+--------------------------------------------------+
|  Runner                                          |
|   - load tazuna.yaml / expand includes           |
|   - verify context_matches / filter by tags      |
|   - resolve dependsOn into parallel layers       |
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
|  genesissecret      |
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

サブコマンド（`apply`, `build`, `check`, `destroy`, `plan`, `status`,
`state list`, `state diff`, `state drift`, `secret-to-genesissecret`, `version`
など）の入り口です。
フラグの解釈と Runner の組み立てだけを担当し、ロジック自体は持ちません。

### Runner

Tazuna 全体のオーケストレータです。次を担当します。

- `tazuna.yaml` のロード
- `includes` の展開
- `tazuna.yaml` 起点の相対パスを実行時パスへの変換
- `--tags` によるフィルタ
- `dependsOn` を解析して並列実行可能な層に分割
- 各 manifest を対応する Manager へ渡す（同層内は goroutine で並列実行）
- 適用後の Test plugin の実行
- apply 成功後の State ConfigMap への書き戻し

Runner は manifest type の種類を知りません。
「どの type に対してどの Manager を使うか」というマップだけを持ち、
それ以外は Manager 側に委ねます。
`dependsOn` の詳細は [`dependsOn` による DAG 実行](./depends-on.md) を参照してください。

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
保存先はクラスタ内の ConfigMap で、`apply` の成功時に書き込まれ、
`state list` / `state diff` / `state drift` / `status` がここを起点に動きます。

### Secret provider

`GenesisSecret` および helmfile の `vars.op` から参照される、
「シークレットの取得元」を抽象化したコンポーネントです。
組み込みで 1Password (`onepassword`) と envfile (`envfile`) の 2 種類を提供します。
`tazuna.yaml` の `spec.providers[]` で複数の provider を宣言し、
GenesisSecret 側の `spec.provider` で名前指定して使い分けられます。
詳細は [Secret provider](../reference/secret-providers.md) を参照してください。

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
5. `dependsOn` を解析して並列実行可能な層に分割する（未使用なら宣言順 1 件ずつ）。
6. 各層内の `manifests` について `--tags` で除外されないものを
   対応する **Manager** に goroutine で並列に渡す。
7. 各 Manager は内部で kustomize / helmfile / oras pull / 1Password 取得 などを呼び出し、
   結果をクラスタへ反映する。
8. manifest 単位 / 全体の Tests があれば **Test plugin** が走る。
9. apply とテストが成功した manifest の指紋を **State store** に書き戻す。

`tazuna apply --sync` はこの 1〜5 までを使ったうえで、各 Manager の `Build` 結果から
**State diff** を組み、追加分・変更分だけを apply します。`--prune` を付けると、
State にあって Build に無いリソースの削除も行います。
