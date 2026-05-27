# Test plugin

Test plugin は、Manifest の適用前後にクラスタの状態を検証するための仕組みです。
`tazuna.yaml` に書き、`tazuna apply` のフローに組み込まれます。

このページでは Test plugin の YAML スキーマと、組み込みの 2 種（`WaitUntil` / `ExistNonExist`）
の仕様をまとめます。

## 配置と実行タイミング

Test plugin は `tazuna.yaml` の 2 箇所に書けます。

| 配置箇所               | 実行タイミング                                  |
|------------------------|-------------------------------------------------|
| `manifests[].tests`    | その Manifest を `apply` した **直後** に実行   |
| `spec.tests`           | **すべての Manifest の apply が終わったあと** に実行 |

`tazuna build` / `tazuna check` / `tazuna state diff` などクラスタを変更しない
コマンドでは Test plugin は実行されません。`tazuna destroy` 時にも実行されません。

## TestPluginSpec（共通フィールド）

`manifests[].tests` と `spec.tests` の各要素は同じ構造 `TestPluginSpec` を取ります。

| フィールド                     | 型     | 必須 | デフォルト | 説明 |
|--------------------------------|--------|------|------------|------|
| `type`                         | string | ◯    | -          | プラグイン種別。`WaitUntil` または `ExistNonExist`（大文字小文字をそのまま）。 |
| `waitUntil`                    | [WaitUntilArgs](#waituntilargs)    | △ (※) | `null` | `type: WaitUntil` のときに必須。 |
| `existNonExist`                | [ExistNonExistArgs](#existnonexistargs) | △ (※) | `null` | `type: ExistNonExist` のときに必須。 |
| `minConsecutiveSuccessCount`   | int    | -    | `1`        | テスト関数が連続でこの回数だけ成功したら、テストプラグイン全体を **成功** とする。`0` を指定したときも内部で `1` に補正されます。 |
| `minConsecutiveFailureCount`   | int    | -    | `0`        | テスト関数が連続でこの回数だけ失敗したら、テストプラグイン全体を **失敗** として打ち切る。`0` のときはこのチェックは行われず、`timeoutSeconds` のみが打ち切り条件になります。 |
| `timeoutSeconds`               | int    | -    | 実質無限   | 全体タイムアウト秒。指定時間を超えると失敗。`0`（未指定）のときは実質無期限（内部で約 280 日が設定されます）。 |
| `intervalSeconds`              | int    | -    | `0`        | テスト関数の再実行間に挟む待機秒。`0` のときは即座に再実行されます。 |

(※) `type` と `waitUntil` / `existNonExist` の対応関係は実行時に検証されます。
`waitUntil` を指定して `type: ExistNonExist` を書く、といった食い違いがあると実行時にエラーになります。

### 評価ループの挙動

すべての Test plugin は次のループで動きます。

1. テスト関数（プラグイン固有のロジック）を 1 回実行する。
2. 結果（成功/失敗）を履歴に追記する。
3. 直近 `minConsecutiveSuccessCount` 件がすべて成功なら **成功** として終了。
4. `minConsecutiveFailureCount` が 0 でなく、直近の `minConsecutiveFailureCount` 件が
   すべて失敗なら **失敗** として終了。
5. `timeoutSeconds` が経過したら **失敗** として終了。
6. `intervalSeconds` 秒スリープして 1 に戻る（`0` なら即時）。

「1 回成功すれば OK」「再試行間隔は 1 秒」「最大 60 秒で打ち切り」を表現したい場合は次のようになります。

```yaml
tests:
  - type: WaitUntil
    timeoutSeconds: 60
    intervalSeconds: 1
    waitUntil:
      # ...
```

## WaitUntilArgs

指定したリソースが「目的の状態」になるまでループで待つプラグインです。
判定条件は CEL 式で表現します。

| フィールド             | 型     | 必須 | 説明 |
|------------------------|--------|------|------|
| `resource.apiVersion`  | string | ◯    | 対象リソースの apiVersion。例: `apps/v1`、`cert-manager.io/v1`。 |
| `resource.kind`        | string | ◯    | 対象リソースの kind。例: `Deployment`、`Certificate`。 |
| `namespace`            | string | ◯    | 対象リソースの namespace。 |
| `name`                 | string | ◯    | 対象リソースの name。 |
| `condition`            | string | ◯    | 真偽を返す CEL 式。式の中では `object` という名前で、取得したリソースが unstructured な map として参照できます。式の評価結果は **bool** 型でなければなりません（コンパイル時に型チェックされます）。 |

各イテレーションは「リソースを `Get` → CEL 式を評価」の組み合わせで動きます。
リソースの取得に失敗した（404 を含む）回もそのループの「失敗」として扱われます。

代表的な `condition` 例:

```yaml
# Deployment が要求どおり Ready になっている
condition: "object.status.readyReplicas == object.spec.replicas"

# Available conditions が True になっている（条件 list 評価）
condition: >-
  object.status.conditions.exists(c,
    c.type == "Available" && c.status == "True")
```

CEL 式自体の言語仕様は CEL の公式ドキュメントを参照してください。
Tazuna は `object` 変数の追加と「結果は bool」という制約だけを掛けています。

### 例（WaitUntil）

```yaml
tests:
  - type: WaitUntil
    timeoutSeconds: 120
    intervalSeconds: 2
    waitUntil:
      resource:
        apiVersion: apps/v1
        kind: Deployment
      namespace: ingress-nginx
      name: ingress-nginx-controller
      condition: "object.status.readyReplicas == object.spec.replicas"
```

## ExistNonExistArgs

指定したリソースが「存在しているべき」または「存在していないべき」を表明するプラグインです。

| フィールド             | 型     | 必須 | 説明 |
|------------------------|--------|------|------|
| `resource.apiVersion`  | string | ◯    | 対象リソースの apiVersion。 |
| `resource.kind`        | string | ◯    | 対象リソースの kind。 |
| `namespace`            | string | ◯    | 対象リソースの namespace。 |
| `name`                 | string | ◯    | 対象リソースの name。 |
| `shouldExist`          | bool   | ◯    | `true` のとき、リソースが存在すれば成功。`false` のとき、存在しなければ成功。 |

判定は 1 回の `Get` 結果で行われます。
`Get` が `NotFound` を返すかどうかで存在判定し、`shouldExist` と突き合わせます。
`NotFound` 以外のエラー（権限不足など）はそのイテレーションの「失敗」として扱われます。

### 例（ExistNonExist）

```yaml
tests:
  # 想定したリソースが入ったことの表明
  - type: ExistNonExist
    existNonExist:
      resource:
        apiVersion: apps/v1
        kind: Deployment
      namespace: tazuna-managed
      name: nginx
      shouldExist: true

  # 廃止したリソースが残っていないことの表明
  - type: ExistNonExist
    timeoutSeconds: 10
    intervalSeconds: 1
    existNonExist:
      resource:
        apiVersion: v1
        kind: Secret
      namespace: tazuna-managed
      name: legacy-token
      shouldExist: false
```

## 関連

- 配置先のフィールド: [`tazuna.yaml` の `tests` フィールド](./tazuna-yaml.md#tests-フィールド)
- 用語: [Test plugin](../concepts/glossary.md#test-plugin)
- 全体アーキテクチャ上の位置付け: [全体アーキテクチャ - Test plugin](../concepts/architecture.md#test-plugin)
