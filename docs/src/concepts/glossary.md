# 用語集

このセクションで頻出する用語の定義をまとめます。
より詳しい仕様は [リファレンス](../reference/index.md) を参照してください。

## Tazuna 固有の用語

### tazuna.yaml
Tazuna に対する唯一の入力ファイル。`apiVersion: tazuna.pepabo.com/v1`,
`kind: Tazuna` を持ち、`spec.manifests[]` に「適用したい manifest 群」を宣言する。

### Manifest
`tazuna.yaml` 内の `manifests[]` の 1 エントリのこと。
`name` / `type` / `path` を持ち、`type` ごとに対応する Manager が処理する。
Kubernetes における「マニフェスト」（YAML ファイル）とは指す対象が違う点に注意。

### Manifest type
Manifest の処理方法を指定する文字列。
`kustomize` / `helmfile` / `genesissecret` / `oras` の 4 種類。
以前あった `parallel` は `dependsOn` ベースの DAG 実行へ置き換えられて削除されています。

### Manager
ある Manifest type に対する処理を担うコンポーネント。
`Apply`（クラスタへの反映）/ `Destroy`（取り外し）/ `Build`（クラスタに触らない生成）の
3 つの操作を提供し、Runner はこの 3 つを介してすべての backend を均一に扱う。

### Runner
Tazuna の中心オーケストレータ。`tazuna.yaml` のロード、`includes` 展開、`--tags` フィルタ、
Manager の呼び出し、Test plugin の起動などをまとめて担当する。

### Test plugin
Manifest 適用の前後で行いたい検証を表現する仕組み。
組み込みで `wait-until`（Ready/存在まで待つ）と `exist-nonexist`（存在/非在の表明）がある。
`spec.manifests[].tests` か `spec.tests` に書く。

### State
Tazuna が「自分が入れたリソース」を追跡するための記録。
クラスタ内 `tazuna` namespace の ConfigMap (`tazuna-state-<manifest-name>`) に保存される。

### State key
State の 1 エントリを指すキー文字列。
namespaced なら `{manifest}/{group}/{version}/{kind}/{namespace}/{name}`、
cluster-scoped なら `{manifest}/{group}/{version}/{kind}/{name}`。

### ContentHash
State の各エントリが持つ SHA-256 ハッシュ値。
リソース YAML から `metadata.resourceVersion` / `uid` / `creationTimestamp` /
`generation` / `managedFields` / `selfLink` と `status` を除いて計算する。
このハッシュの一致／不一致が `state diff` の判定基準。

### Diff type
`tazuna state diff` がリソースごとに付ける分類。
`added` / `modified` / `removed` / `always-sync` の 4 種類。

### always-sync
GenesisSecret 由来 Secret のように、差分計算をスキップして毎回同期する扱いの Diff type。
ContentHash で変化を判定できない / させない対象に使う。
1Password 側で値が更新されてもクラスタ側のハッシュは変わらないため、
ContentHash を使わずに毎回 Provider に問い合わせる形で同期する設計になっている。
利用例と運用上の扱いは
[GenesisSecret スキーマ - State と always-sync](../reference/genesis-secret.md#state-と-always-sync)
と [Drift モニタリング](../operations/drift-monitoring.md) を参照。

### GenesisSecret
1Password に保管された秘密情報から、Kubernetes Secret を **生成** するための宣言。
Kubernetes CRD ではなく、Tazuna が読む YAML スキーマ。
`type: genesissecret` の manifest として `tazuna.yaml` から参照する。

### Provider (SecretProvider)
GenesisSecret が秘匿情報を取り出す元を抽象化したインターフェース。
組み込みで 1Password (`onepassword`) と envfile (`envfile`) の 2 種類が利用でき、
`tazuna.yaml` の `spec.providers[]` で複数宣言・GenesisSecret 側の `spec.provider` で
名前指定して使い分けられる。詳細は [Secret provider](../reference/secret-providers.md) を参照。

### `default-op`
組み込みの 1Password provider に予約された名前。
GenesisSecret の `spec.provider` が空文字のときに後方互換のためフォールバックされる。
`spec.providers[]` でこの名前を再宣言することはできない。

### `dependsOn`
`manifests[]` のエントリで、その Manifest を apply する前に完了している必要がある
Manifest 名のリスト。`tazuna.yaml` 内に 1 つでも `dependsOn` が書かれていれば
Runner は DAG モードに切り替わり、同じ依存深度の Manifest を並列に実行する。
詳細は [`dependsOn` による DAG 実行](./depends-on.md) を参照。

### Layer (DAG 層)
`dependsOn` を解決した結果として導出される、**並列に実行可能な Manifest の集合**。
同一層に属する Manifest は互いに依存しておらず、Runner が goroutine で並列に
apply する。層単位の境界では待ち合わせが入るため、層をまたいだ順序保証は崩れない。

### `live-drifted` / `live-missing`
`tazuna state drift` が出力する drift 分類。`live-drifted` は State の hash と
ライブクラスタの hash が一致しないリソース、`live-missing` は State に記録は
あるがライブクラスタから NotFound のリソース。

### `context_matches`
`tazuna.yaml` の `spec.context_matches`。
現在の kubeconfig context 名がマッチすべき正規表現の配列で、
誤クラスタへの apply を防ぐためのガード。

### `context_match_mode`
`context_matches` の評価方式。`or`（デフォルト）または `and`。

### includes
`manifests[]` のエントリで、別ファイルの `tazuna.yaml` を読み込み、
その `manifests[]` を展開するための仕組み。ネストは不可。

### Tag (manifest tag)
`manifests[].tags` に書く文字列。`tazuna apply --tags foo,bar` のように
適用対象を絞り込むときに使う。複数タグは **OR** で評価される。

### Manifest path
`manifests[].path`。`tazuna.yaml` 自身の置かれているディレクトリ起点の相対パスとして書く。
Tazuna は実行時に cwd 起点へ変換する。

### `tazuna.hint.yaml`
helmfile の `vars` が取りうる値の型・フォーマット制約を宣言するヒントファイル。
`pkg/hint/` で読まれる。

## Kubernetes 側の用語（補足）

ここに挙げるのは Tazuna 内で頻出する Kubernetes 標準用語の短い定義です。

### kubeconfig
クラスタ接続情報（cluster / user / context）をまとめた YAML。
Tazuna はここから `current-context` を読み、そのクラスタへ操作を行う。

### context (kubeconfig context)
「どの cluster にどの user で繋ぐか」を 1 名前にまとめた kubeconfig の要素。
`context_matches` が正規表現でチェックするのはこの context 名。

### GVK (Group/Version/Kind)
Kubernetes リソースの種別を一意に特定する 3 つ組。
Tazuna の State key も GVK を含む。

### namespaced / cluster-scoped
リソースが namespace に属するか属さないか。
State key の長さ（5 パートか 6 パート）に反映される。

### ConfigMap
任意の key-value をクラスタに保存するための組み込みリソース。
Tazuna の State の保存先として使われる。

## 外部ツール

### kustomize
Kubernetes manifest の overlay / patch 機構。
`type: kustomize` の Manager から呼び出される。

### helmfile
複数の Helm release を 1 YAML から束ねるツール。
`type: helmfile` の Manager から呼び出される。

### Helm
Helm chart を扱うパッケージマネージャ。helmfile から内部的に使われる。

### ORAS / OCI artifact
OCI registry に Kubernetes manifest のようなコンテナ以外の成果物を置く規格。
`type: oras` で pull し、`delegate` に指定した helmfile / kustomize に処理を委譲する。

### 1Password
Tazuna が `GenesisSecret` や `helmfile.vars.op` で参照する Secret の格納先。
取得は `op` コマンド経由。
