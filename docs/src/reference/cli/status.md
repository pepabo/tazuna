# `tazuna status`

State ConfigMap に記録されている **managed リソース** を順に取得し、
それぞれの readiness を一覧表示します。
クラスタへの read アクセスのみを行い、何も変更しません。

```text
tazuna status [-f tazuna.yaml]
```

## 振る舞い

1. `tazuna.yaml` をロードする。
2. 各 Manifest について、対応する State ConfigMap を読む。
3. State に記録されている各リソースを GVK / namespace / name で
   クラスタから取得する。
4. 取得結果から readiness を判定し、3 カラムで出力する。

`context_matches` の評価は行いません。

## 判定基準

| ステータス  | 判定基準 |
|-------------|----------|
| `Ready`     | 実体が取得でき、Kind に応じた Ready 判定に通った |
| `NotReady`  | 実体は取得できたが、Ready 判定に通らなかった |
| `Missing`   | State には記録されているが、ライブクラスタで NotFound |
| `Error`     | NotFound 以外のエラーが取得時に発生した（権限不足、API server エラー等） |

### Kind ごとの Ready 判定

Ready 判定は Kind に応じて分岐します。**Deployment / StatefulSet / DaemonSet / Pod
以外の Kind は、取得できた時点で即 `Ready` 扱い** になります（ConfigMap / Secret /
Service / Ingress / CRD など、readiness の概念を持たないリソースを画一的に扱うため）。

| Kind          | Ready の条件 |
|---------------|--------------|
| `Deployment`  | `spec.replicas == 0` で即 Ready。それ以外は `status.readyReplicas == status.replicas == status.availableReplicas` かつ `replicas > 0` |
| `StatefulSet` | `spec.replicas == 0` で即 Ready。それ以外は `status.readyReplicas == status.replicas` かつ `replicas > 0` |
| `DaemonSet`   | `status.numberReady == status.desiredNumberScheduled` かつ `desiredNumberScheduled > 0` |
| `Pod`         | `status.phase == "Running"` かつ `Ready` condition が `True` |
| その他         | 取得できた時点で `Ready` |

readiness 概念を本来持たない Service / ConfigMap 等を `NotReady` で塗りつぶしてしまわない
ための設計です。Ingress / CRD / Custom Resource 等で固有の readiness を見たい場合は、
別途 [Test plugin](../test-plugin.md) の `WaitUntil` を CEL で書くのが筋となります。

## 出力フォーマット

3 カラム（STATUS / KIND / NAMESPACE/NAME）で出します。cluster-scoped リソースは
NAMESPACE/NAME 列に namespace が付かず NAME のみが出ます。

```text
Manifest: ingress-nginx
  STATUS    KIND                 NAMESPACE/NAME
  Ready     Deployment           ingress-nginx/controller
  Ready     Service              ingress-nginx/controller
  Ready     ConfigMap            ingress-nginx/controller
  NotReady  Deployment           ingress-nginx/admission-webhook
  Missing   Secret               ingress-nginx/missing-tls

Manifest: cloud-credentials
  (no state)
```

State がまだ作られていない（一度も apply / sync されていない）Manifest は
`(no state)` と表示されます。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) 以外に固有フラグはありません。

## 例

```bash
tazuna status
tazuna status -f tazuna.yaml
tazuna status -f tazuna.yaml --otlp-endpoint=localhost:4317
```

## 関連

- 宣言 vs State の差分は [`tazuna state diff`](./state-diff.md)
- 手動変更の検知は [`tazuna state drift`](./state-drift.md)
- 適用は [`tazuna apply`](./apply.md)
- 用語: [State](../../concepts/glossary.md#state)
