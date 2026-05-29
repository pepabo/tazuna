# `tazuna state drift`

State ConfigMap に記録されたリソースと、**ライブクラスタ上の実体** を突き合わせて、
手動で変更されたリソース（`live-drifted`）や、State には残っているが
ライブから取り除かれたリソース（`live-missing`）を検知します。
クラスタへの read アクセスのみを行い、何も変更しません。

```text
tazuna state drift [-f tazuna.yaml]
```

## 振る舞い

1. `tazuna.yaml` をロードする。
2. 各 Manifest について、対応する State ConfigMap（`tazuna` namespace の
   `tazuna-state-<manifest-name>`）を読む。
3. State に記録された各リソースを GVK / namespace / name で **クラスタから取得** する。
4. 取得できたリソースについて [ContentHash](../../concepts/glossary.md#contenthash) を
   再計算し、State に保存されたハッシュと突き合わせる。
5. 一致しないものを `live-drifted`、取得自体に失敗（NotFound）したものを
   `live-missing` として出力する。

`context_matches` の評価は行いません。
`tazuna state diff` と異なり、**Build 結果は使いません**。State に記録された
ハッシュとライブの実体を比較するだけです。

## `state diff` との違い

| 観点                | `state diff`                       | `state drift`                      |
|---------------------|------------------------------------|------------------------------------|
| 比較対象            | Build 結果 vs State                | State vs ライブクラスタ            |
| クラスタ read       | State ConfigMap のみ               | State + 各リソース実体             |
| 検出できるもの      | `tazuna.yaml` 側の変更             | 手動 `kubectl apply` / 手動削除    |
| GenesisSecret       | 常に `always-sync`                 | スキップ                           |

`state diff` は「`tazuna.yaml` を更新したのにまだ反映していない」を検知し、
`state drift` は「`tazuna.yaml` は変えていないのに、誰かがクラスタを触った」を
検知する、と覚えると棲み分けが分かりやすくなります。

## 出力フォーマット

drift が見つかった場合、Manifest 単位で次のように出力します。

```text
Manifest: ingress-nginx
  STATUS         RESOURCE                                                     HASH
  live-drifted   ingress-nginx/apps/v1/Deployment/ingress-nginx/controller    def456... (stored: abc123...)
  live-missing   ingress-nginx//v1/ConfigMap/ingress-nginx/extra              (stored: zzz999...)
```

drift が無い場合は次の 1 行だけが出ます。

```text
No drift detected.
```

`state drift` 自体は drift の有無で **終了コードを変えません**。CI で「drift あり」
を失敗扱いにしたい場合は、出力に `No drift detected.` を含まないかどうかでフィルタします。

## スキップされる Manifest

次の Manifest は drift 検知対象外として **スキップ** されます。

- `name` が空の Manifest（State key を作れないため）
- `type: genesissecret`（[always-sync](../../concepts/glossary.md#always-sync) 設計上、
  毎回 Provider から再取得するためそもそも drift という概念を持たない）
- State ConfigMap が空または存在しない Manifest（一度も apply / sync されていない）

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) 以外に固有フラグはありません。

## 例

```bash
tazuna state drift
tazuna state drift -f tazuna.yaml
tazuna state drift -f tazuna.yaml --otlp-endpoint=localhost:4317
```

## KinD での再現手順

手元で `live-drifted` を再現するには次の手順がもっとも簡単です。

```bash
# 1. 普通に適用して state を作る
tazuna apply -f tazuna.yaml

# 2. クラスタ側のリソースを手で変更する
kubectl -n default patch deployment nginx --type=merge \
  -p '{"spec":{"replicas":3}}'

# 3. drift を検知する
tazuna state drift -f tazuna.yaml
# => live-drifted エントリが 1 件出る
```

`live-missing` は手で `kubectl delete` すると同様に再現できます。

## 関連

- 宣言 vs State の差分は [`tazuna state diff`](./state-diff.md)
- drift モニタリングの組み立て方は [Drift モニタリング](../../operations/drift-monitoring.md)
- 用語: [State](../../concepts/glossary.md#state) /
  [ContentHash](../../concepts/glossary.md#contenthash)
