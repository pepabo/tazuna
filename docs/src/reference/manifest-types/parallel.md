# `type: parallel`

`parallel` Manifest は、複数の子 Manifest を **並列に処理する** ためのコンテナです。
子の `type` は `kustomize` / `helmfile` / `genesissecret` / `oras` のいずれか
（`parallel` のネストは想定していません）。

## `path`

`parallel` Manifest 自体の `manifests[].path` は **実体としては使用されません**。
バリデーション都合で空にはできないため、何かしらのディレクトリを書きます。

実際の処理対象パスは、各 `children[].path` 側で指定します。

## 固有フィールド

`manifests[].parallel` のオブジェクトに書きます。

| フィールド   | 型           | 必須 | デフォルト | 説明 |
|--------------|--------------|------|------------|------|
| `children`   | [Manifest]   | ◯    | -          | 並列に処理する子 Manifest の配列。**1 件以上** 必要。 |

`children[]` の各要素は [Manifest](../tazuna-yaml.md#manifest) と同じ構造です。
`children[]` 内の Manifest が持つ `name` も、include 展開後の全 Manifest 名と同じ空間で
一意でなければなりません（`tazuna check` で検証されます）。

## 振る舞い

| 操作      | 内部処理 |
|-----------|----------|
| `Apply`   | `children[]` の各要素に対応する Manager の `Apply` を **goroutine で並列** に呼ぶ。エラーは集約して返す。 |
| `Destroy` | `children[]` の各要素に対応する Manager の `Destroy` を並列に呼ぶ。 |
| `Build`   | `children[]` の各要素に対応する Manager の `Build` を並列に呼び、宣言順を保ったまま `\n---\n` で結合した文字列を返す。空文字の出力はスキップ。 |

`children[]` の処理順序は保証されません。並列で動かして問題ないグループにだけ使ってください。
順序依存（A の CRD を待ってから B を入れる、など）がある場合は、`parallel` を使わず
通常の `manifests[]` の宣言順に並べるか、子 Manifest 側の [Test plugin](../test-plugin.md) で
Ready 待ちを表現します。

## 例

並列に入れて構わない 2 つの kustomize を 1 つの parallel に束ねる例:

```yaml
manifests:
  - name: observability
    type: parallel
    path: ./parallel/observability   # 実体としては使われないが必須
    parallel:
      children:
        - name: prometheus
          type: kustomize
          path: ./kustomize/prometheus
          tags:
            - observability
        - name: grafana
          type: kustomize
          path: ./kustomize/grafana
          tags:
            - observability
```

## 関連

- 子 Manifest として書ける type: [`kustomize`](./kustomize.md) /
  [`helmfile`](./helmfile.md) / [`genesissecret`](./genesissecret.md) /
  [`oras`](./oras.md)
- Manifest 共通フィールド: [`tazuna.yaml` - Manifest](../tazuna-yaml.md#manifest)
