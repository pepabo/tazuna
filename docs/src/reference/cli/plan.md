# `tazuna plan`

`tazuna.yaml` で宣言された Manifest を Build した結果と、**ライブクラスタの状態** を
比較し、apply したらどう変わるかをフィールド単位で出力します。
クラスタへの read アクセスのみを行い、何も変更しません。

```text
tazuna plan [-f tazuna.yaml] [--tags ...]
```

## 振る舞い

1. `tazuna.yaml` をロードする。
2. `--tags` でフィルタする。
3. 各 Manifest を対応する Manager の `Build()` でレンダリングする。
4. レンダリング結果を `client.Object` 群に変換する。
5. 各オブジェクトを GVK / namespace / name で **クラスタから取得** する。
6. 取得できなかった（NotFound）オブジェクトは `+ to be created` 扱いで出力する。
7. 取得できたオブジェクトは desired と live を unified diff にして `~` で出力する。

`context_matches` の評価は行いません。

## なぜ client-side diff なのか

Tazuna の `plan` は「server-side dry-run」というスローガンの下で実装されていますが、
実装は **client-side diff** です。これは次のトレードオフの結果です。

| 方式                  | メリット                              | デメリット                         |
|-----------------------|---------------------------------------|------------------------------------|
| server-side dry-run   | admission webhook / defaulting 反映   | controller-runtime の fake client が dry-run apply を完全サポートしない |
| client-side diff (採用) | integration test で再現可能           | webhook / defaulting の結果は見えない |

正確性を多少犠牲にしてでも「テストできる plan」を取った、と理解してください。
admission webhook で書き換えられるフィールド（mutation）や server-side defaulting は
plan の出力には反映されません。

## 出力フォーマット

```text
Manifest: nginx
  + Deployment/default/nginx-new (to be created)
  ~ ConfigMap/default/nginx-conf
        spec:
          replicas: 1
    +     replicas: 3

Manifest: cert-manager
  + Issuer/cert-manager/letsencrypt-prod (to be created)
```

- `+ <Kind/ns/name> (to be created)` — ライブクラスタにまだ存在しないリソース
- `~ <Kind/ns/name>` — 存在するがフィールド差分があるリソース。直下に
  `k8s.io/apimachinery/pkg/util/diff.Diff` の unified diff がインデント付きで続く

差分が無い場合は `No changes detected.` の 1 行だけが出ます。

### 差分計算時に除外されるフィールド

ノイズを避けるため、live と desired の比較前に次のフィールドは取り除かれます。

- `metadata.resourceVersion` / `uid` / `generation`
- `metadata.managedFields`
- `metadata.creationTimestamp` / `selfLink`
- `status`

## スキップされる Manifest

- `name` が空の Manifest
- `type: genesissecret`（[always-sync](../../concepts/glossary.md#always-sync) のため
  plan のフィールド diff という概念に合わない）

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) に加えて次を受け付けます。

| フラグ    | エイリアス | 型       | デフォルト | 説明 |
|-----------|------------|----------|------------|------|
| `--tags`  | `-t`       | []string | `[]`       | 指定タグのいずれかが付いている Manifest だけを plan 対象にします（OR 評価）。 |

## 例

```bash
tazuna plan
tazuna plan -f tazuna.yaml
tazuna plan -f tazuna.yaml --tags web,batch
tazuna plan -f tazuna.yaml --otlp-endpoint=localhost:4317
```

## 関連

- 実際に反映するときは [`tazuna apply`](./apply.md)
- State 起点の差分は [`tazuna state diff`](./state-diff.md)
- 手動変更の検知は [`tazuna state drift`](./state-drift.md)
