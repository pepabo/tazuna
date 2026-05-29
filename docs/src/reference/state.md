# State の内部構造

State は、Tazuna が「自分が入れたリソース」を追跡するための記録です。
クラスタ内の **ConfigMap** に保存され、`tazuna state list` / `tazuna state diff` /
`tazuna state drift` / `tazuna status` / `tazuna plan` の起点になります。
書き込みは `tazuna apply` の成功時に自動で行われます（`--sync` 有無を問わず）。

このページでは State の保存形式と、それを支える State key / ContentHash / Diff type の
仕様をまとめます。

## 保存場所

State は **クラスタ内** に保存されます。Tazuna のローカルファイルやリモートストレージは使いません。

| 要素           | 値                                |
|----------------|-----------------------------------|
| Namespace      | `tazuna`（無ければ自動作成される） |
| ConfigMap 名   | `tazuna-state-<manifest-name>`    |
| 形式           | `ConfigMap.data` に key/value で 1 リソース 1 エントリ |

1 つの Manifest が 1 つの ConfigMap に対応します。
`includes` 展開後の各 Manifest 名はユニーク（[`tazuna.yaml` の `name`](./tazuna-yaml.md#name) 参照）なので、
ConfigMap 名は衝突しません。

## State key

State key は、ConfigMap 内の 1 エントリを指す識別子です。
構造体としては `manifestName` / `group` / `version` / `kind` / `namespace` / `name` を持ち、
文字列化のフォーマットは次の 2 通りです。

| リソースのスコープ | 形式                                                            | パート数 |
|--------------------|-----------------------------------------------------------------|----------|
| namespaced         | `{manifest}/{group}/{version}/{kind}/{namespace}/{name}`        | 6        |
| cluster-scoped     | `{manifest}/{group}/{version}/{kind}/{name}`                    | 5        |

`group` は core group（`""`）の場合は空セグメントになります。
例: core/v1 の `ConfigMap`（namespaced）であれば、`my-manifest//v1/ConfigMap/default/my-cm` のように
2 つ目のスラッシュ間が空になります。

### ConfigMap data key へのエンコード

Kubernetes の `ConfigMap.data` の key は `[-._a-zA-Z0-9]+` しか許さず、`/` を含められません。
そのため Tazuna は ConfigMap への書き込み時に State key 文字列の `/` を `__` に置換します。
読み込み時には逆変換します。

```text
state key:    nginx/apps/v1/Deployment/default/nginx
data key:     nginx__apps__v1__Deployment__default__nginx
```

Kubernetes の DNS-1123 名（manifest 名 / group / namespace / name）に `_` は含まれないため、
`__` は安全なセパレータとして使えます。

## State エントリ (`StateEntry`)

各エントリは ConfigMap 上 1 つの value として、次の JSON 形で書き込まれます。

```json
{"contentHash":"<hex sha256>"}
```

| フィールド    | 型     | 説明 |
|---------------|--------|------|
| `contentHash` | string | リソースの内容に対する SHA-256 ハッシュ。詳細は [ContentHash](#contenthash) 参照。 |

## `_metadata` キー

ConfigMap には予約キー `_metadata` がもう 1 つ入ります。
これは State エントリではなく、State 全体のメタ情報です。

```json
{"gitCommitHash":"<sha>","lastSyncedAt":"<rfc3339>"}
```

| フィールド       | 型     | 説明 |
|------------------|--------|------|
| `gitCommitHash`  | string | 同期時に記録された git commit hash。 |
| `lastSyncedAt`   | string | 同期時刻。 |

Manifest の `name` には `_metadata` は使えません
（[`tazuna.yaml` の `name`](./tazuna-yaml.md#name) で予約語として扱われています）。
この衝突を避けるためのガードです。

## ContentHash

ContentHash は、各リソースの YAML 表現から計算される SHA-256 の hex 文字列です。
Tazuna は **server-side で付与される field** と **`status`** を除いて計算します。

除外するフィールド:

| フィールド                    | 除外理由 |
|-------------------------------|----------|
| `metadata.resourceVersion`    | クラスタの世代管理用、適用ごとに変動するため |
| `metadata.uid`                | クラスタ採番、適用ごとに変動するため |
| `metadata.creationTimestamp`  | クラスタ採番、適用ごとに変動するため |
| `metadata.generation`         | クラスタ採番、適用ごとに変動するため |
| `metadata.managedFields`      | server-side apply の追跡情報 |
| `metadata.selfLink`           | クラスタ採番 |
| `status`                      | コントローラが書く動的フィールド |

計算手順:

1. オブジェクトを JSON マーシャル / アンマーシャルして deep copy する。
2. 上記の除外対象フィールドを削除する。
3. 残りを JSON マーシャルする。
4. SHA-256 を取り hex 文字列化する。

`status` を含めると Pod の再起動などで毎回ハッシュが変わってしまうので、
**「`tazuna.yaml` から見て同じ状態か」** を判定できる粒度に絞っています。

## Diff type

`tazuna state diff` と `tazuna apply --sync` は、`Build` 結果と既存 State の比較結果を
**Diff type** で分類します。

| Diff type     | 意味                                                    | `apply --sync` の挙動 |
|---------------|---------------------------------------------------------|-----------------------|
| `added`       | Build 結果に存在し、State に無い                        | 反映する              |
| `modified`    | 両方に存在するが、ContentHash が異なる                  | 反映する              |
| `removed`     | State に存在し、Build 結果に無い                        | デフォルトはスキップ。`--prune` のときだけ削除 |
| `always-sync` | 差分計算をスキップし、常に同期扱いとする                 | 反映する              |

`state diff` の出力は **`added` → `modified` → `removed` → `always-sync`** の順、
同 Diff type 内は State key 昇順で安定ソートされます。

### `always-sync` の対象

現バージョンでは、`type: genesissecret` 由来の Secret が `always-sync` 分類になります。
これらは Provider 側を真実の源として毎回同期する性質を持ち、ContentHash で差分判定できる対象ではありません。
詳細は [GenesisSecret スキーマ - State と always-sync](./genesis-secret.md#state-と-always-sync) を参照してください。

## 関連

- 操作する CLI: [`tazuna state list`](./cli/state-list.md) /
  [`tazuna state diff`](./cli/state-diff.md) /
  [`tazuna state drift`](./cli/state-drift.md) /
  [`tazuna status`](./cli/status.md) /
  [`tazuna plan`](./cli/plan.md) /
  [`tazuna apply --sync`](./cli/apply.md#state-連携---sync----prune----atomic)
- 用語: [State](../concepts/glossary.md#state) /
  [State key](../concepts/glossary.md#state-key) /
  [ContentHash](../concepts/glossary.md#contenthash) /
  [Diff type](../concepts/glossary.md#diff-type) /
  [always-sync](../concepts/glossary.md#always-sync)
