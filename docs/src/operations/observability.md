# オブザーバビリティ

Tazuna は **OpenTelemetry** ベースのトレーシングを組み込みでサポートします。
すべての CLI コマンドが OTLP/gRPC 経由で trace を export でき、CI / クラスタ運用上の
時間計測やエラー追跡に使えます。

トレーシングは **opt-in** です。何もフラグを渡さなければ no-op tracer が使われ、
外部依存はゼロのままです。

## 有効化

ルートコマンドに次のグローバルフラグが追加されています。

| フラグ              | 型     | デフォルト | 説明 |
|---------------------|--------|------------|------|
| `--otlp-endpoint`   | string | `""`       | OTLP/gRPC collector のエンドポイント（例: `localhost:4317`）。空文字なら no-op。 |
| `--otlp-insecure`   | bool   | `true`     | OTLP exporter でプレーンテキスト gRPC を使う（TLS 無し）。 |

`--otlp-endpoint` を渡したコマンドだけが trace を発火します。短命 CLI が collector に
ぶら下がってしまわないよう、shutdown には 5 秒のタイムアウトが入っています。

```bash
# Jaeger / Tempo / OTel Collector など、OTLP/gRPC を受けられる先に向ける
tazuna apply -f tazuna.yaml --otlp-endpoint=localhost:4317
tazuna plan  -f tazuna.yaml --otlp-endpoint=localhost:4317
```

## トレースの構造

Tazuna は 3 階層の trace tree を出します。

```
tazuna.Apply / tazuna.Plan / tazuna.Status / tazuna.StateDrift  ← Runner top-level span
  └── tazuna.ApplyToCluster                                      ← Runner internal span
        └── Kustomize.Apply / Helmfile.Apply / GenesisSecret.Apply / ORAS.Apply
                                                                 ← Manager span
```

- **Runner span**（tracer 名 `tazuna/runner`）— トップレベルコマンドの実行時間を
  まとめて測ります。CLI から最初に開く span です。
- **Manager span**（tracer 名 `tazuna/manager`）— 各 Manifest type 固有の処理
  （`kubectl apply` 相当 / `helmfile sync` / oras pull / etc.）を 1 span ずつ計測します。

Runner span と Manager span の名前を分けてあるので、Datadog / Jaeger 等で
service / operation 単位の分析がしやすくなっています。

## 主な span attribute

| Attribute              | 付くタイミング                | 値の例                          |
|------------------------|-------------------------------|---------------------------------|
| `tazuna.yaml.path`     | Runner span                   | `./tazuna.yaml`                 |
| `manifests.count`      | Runner span                   | `12`                            |
| `apply.sync`           | `tazuna.Apply` span           | `true` / `false`                |
| `apply.prune`          | `tazuna.Apply` span           | `true` / `false`                |
| `apply.atomic`         | `tazuna.Apply` span           | `true` / `false`                |
| `manifest.name`        | Manager span                  | `ingress-nginx`                 |
| `manifest.type`        | Manager span                  | `kustomize` / `helmfile` / `oras` / `genesissecret` |
| `manifest.path`        | Manager span                  | `./kustomize/ingress`           |
| `genesissecret.provider` | `GenesisSecret.Apply` span  | `primary-op` / `default-op`     |

エラー時には span に `error` ステータスが付き、メッセージは `span.RecordError` で
ぶら下がります。

## CI での使い方

Datadog / Honeycomb / Grafana Cloud などの SaaS collector を使うと、apply の所要時間や
失敗率を時系列で追えます。CI から渡す例:

```yaml
# GitHub Actions
- name: tazuna apply (with tracing)
  env:
    OTEL_EXPORTER_OTLP_HEADERS: api-key=${{ secrets.OTEL_API_KEY }}
  run: tazuna apply -f tazuna.yaml --otlp-endpoint=otel.example.com:4317
```

short-lived な CLI なので、collector 側で `service.name=tazuna` で絞ると、CI run
ごとの span tree がそのまま見えるはずです。

## 関連

- フラグ仕様: [CLI - グローバルフラグ](../reference/cli/index.md#グローバルフラグ)
- Drift モニタリング: [Drift モニタリング](./drift-monitoring.md)
- CI 連携: [CI パイプライン](./ci-pipeline.md)
