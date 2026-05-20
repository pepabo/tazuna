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
`kustomize` / `helmfile` / `genesissecret` / `parallel` / `oras` の 5 種類。

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

### GenesisSecret
1Password に保管された秘密情報から、Kubernetes Secret を **生成** するための宣言。
Kubernetes CRD ではなく、Tazuna が読む YAML スキーマ。
`type: genesissecret` の manifest として `tazuna.yaml` から参照する。

### Provider (SecretProvider)
GenesisSecret が秘匿情報を取り出す元を抽象化したインターフェース。
現状は 1Password (`op`) 向けの実装が組み込まれている。

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
