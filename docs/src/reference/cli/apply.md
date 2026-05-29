# `tazuna apply`

`tazuna.yaml` で宣言された Manifest 群をクラスタへ反映します。
Tazuna の中心となるコマンドです。

```text
tazuna apply [-f tazuna.yaml] [--tags ...] [--no-cache | --offline]
             [--sync [--prune] [--atomic]]
```

## 振る舞い

実行順序は次のとおりです。クラスタに触れるのは 5 以降です。

1. `tazuna.yaml` をロードしてバリデーションする。
2. `spec.context_matches` が設定されていれば、current-context と照合する。
   合致しなければ即終了する。
3. `--tags` でフィルタする。
4. `manifests[]` を **`dependsOn` から導出した層** に分割する。
   `dependsOn` が 1 つも使われていなければ層数 = manifest 数となり、
   従来の宣言順・順次実行と完全に同じ挙動になる。
5. 各層内のマニフェストを **並列に** 対応 Manager に渡し、クラスタへ反映する。
6. 各 Manifest の apply 成功後、`tests` を実行する。
7. apply とテストが成功した Manifest について **state ConfigMap** に
   content hash を書き戻す（以降の `state diff` / `state drift` / `plan` の起点）。
8. すべての Manifest 適用後、`spec.tests`（全体 Tests）を実行する。

`dependsOn` の挙動は [`dependsOn` による DAG 実行](../../concepts/depends-on.md) を参照してください。

## State 連携 (--sync / --prune / --atomic)

`tazuna apply` は **デフォルトでも state ConfigMap を書き込む** 設計です。
以前は別コマンドだった `tazuna state sync` の役割は、`--sync` フラグを付けた
`apply` に統合されました（後述の「移行: 旧 `state sync` からの置き換え」参照）。

| フラグ        | 振る舞い |
|---------------|----------|
| なし          | すべての Manifest を Manager の `Apply()` 経由で **無条件に** 反映し、その後 state を保存する。 |
| `--sync`      | 差分モードに切り替わる。各 Manager の `Build()` 結果と保存 state を比較し、`added` / `modified` / `always-sync` 分類のリソースだけを `CreateOrUpdate` する。 |
| `--sync --prune` | 上記に加え、**state にあって Build 結果に無い** リソース（`removed`）を `Delete` する。`--sync` 必須。 |
| `--sync --atomic` | 各 Manifest の state 保存を後段にまとめる。途中で 1 つでもエラーが出たら state は **一切更新せず** 終了する。`--sync` 必須。 |

`--prune` / `--atomic` を `--sync` 無しで指定するとバリデーションエラーになります。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) に加えて次を受け付けます。

| フラグ        | エイリアス | 型       | デフォルト | 説明 |
|---------------|------------|----------|------------|------|
| `--tags`      | `-t`       | []string | `[]`       | 指定したタグのいずれかが付いている Manifest だけを処理対象にします（OR 評価）。 |
| `--no-cache`  | -          | bool     | `false`    | `type: oras` の Manifest で、キャッシュを使わずに常に registry から再取得します。 |
| `--offline`   | -          | bool     | `false`    | `type: oras` の Manifest で、registry へのアクセスを禁止します。キャッシュにヒットしなければエラーになります。 |
| `--sync`      | -          | bool     | `false`    | 差分モードを有効化。state と Build 結果の差分のみを反映する。 |
| `--prune`     | -          | bool     | `false`    | state にあって Build 結果に無いリソースを削除する。`--sync` 必須。 |
| `--atomic`    | -          | bool     | `false`    | 全 Manifest 成功後にまとめて state を保存する。途中エラーで state を変更しない。`--sync` 必須。 |

`--no-cache` と `--offline` は同時に指定できません。

## 例

```bash
# 通常の apply (state は自動保存される)
tazuna apply -f tazuna.yaml

# タグで絞り込み
tazuna apply -f tazuna.yaml --tags web,batch

# 差分モード: state と差があるリソースだけを反映
tazuna apply -f tazuna.yaml --sync

# 差分モード + 不要リソースの削除
tazuna apply -f tazuna.yaml --sync --prune

# 差分モード + atomic (途中エラーで state を巻き戻す)
tazuna apply -f tazuna.yaml --sync --atomic

# OTLP/gRPC エンドポイントに trace を流す
tazuna apply -f tazuna.yaml --otlp-endpoint=localhost:4317
```

## 移行: 旧 `tazuna state sync` からの置き換え

以前の Tazuna には `tazuna state sync` という別コマンドがあり、`state` と Build 結果の
差分を反映する役割を担っていました。本リファクタでこのコマンドは **削除** され、
`tazuna apply --sync` へ統合されています。対応関係は次のとおりです。

| 旧コマンド                                            | 新コマンド                                          |
|-------------------------------------------------------|-----------------------------------------------------|
| `tazuna state sync`                                   | `tazuna apply --sync`                               |
| `TAZUNA_STATE_SYNC_DELETE=true tazuna state sync`     | `tazuna apply --sync --prune`                       |
| `tazuna state sync --atomic`                          | `tazuna apply --sync --atomic`                      |

挙動上の違いとして、新しい `apply` は **`--sync` を付けなくても state を保存する** ように
なっています（旧 `apply` は state を一切書きませんでした）。これにより、`apply` 一発で
`state list` / `state diff` / `state drift` / `plan` / `status` が常に意味のある結果を返す
ようになっています。

## 関連

- 評価される [`context_matches`](../tazuna-yaml.md#context_matches)
- フィルタの仕様は [`manifests[].tags`](../tazuna-yaml.md#tags)
- 反映前にレンダリングだけ確認したい場合は [`tazuna build`](./build.md)
- フィールド単位の dry-run diff は [`tazuna plan`](./plan.md)
- 反映後の readiness 確認は [`tazuna status`](./status.md)
- State 差分の参照は [`tazuna state diff`](./state-diff.md) / [`tazuna state drift`](./state-drift.md)
- 既存リソースを取り外す場合は [`tazuna destroy`](./destroy.md)
- DAG 実行モデル: [`dependsOn` による DAG 実行](../../concepts/depends-on.md)
