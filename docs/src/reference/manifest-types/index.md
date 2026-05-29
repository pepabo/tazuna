# Manifest type 別リファレンス

`tazuna.yaml` の `manifests[].type` には 4 種類の値を取れます。
このセクションでは type ごとの **`path` が指す先**、**固有フィールド**、
**apply / destroy / build 時の振る舞い** を 1 ページずつにまとめます。

Manifest の共通フィールド（`name` / `path` / `type` / `tags` / `dependsOn` /
`includes` / `tests`）の仕様は
[`tazuna.yaml` スキーマ - Manifest](../tazuna-yaml.md#manifest) を参照してください。

> 以前あった `type: parallel` は、`dependsOn` ベースの DAG 実行モデルに置き換わるため
> 本リファクタで削除されています。並列に流したい Manifest は通常の Manifest として
> 並べたうえで `dependsOn` を空にしておけば、同じ層に入り並列実行されます。
> 詳細は [`dependsOn` による DAG 実行](../../concepts/depends-on.md) を参照してください。

## 一覧

- [`kustomize`](./kustomize.md) — kustomize でレンダリングしたリソースを反映する
- [`helmfile`](./helmfile.md) — helmfile template の結果を反映する
- [`oras`](./oras.md) — OCI registry から artifact を pull し、helmfile / kustomize に委譲する
- [`genesissecret`](./genesissecret.md) — GenesisSecret YAML から Kubernetes Secret を生成する

## type と固有フィールドの対応

各 type は `manifests[]` 内で **対応するオプションオブジェクト** を持ちます。
`type` と対応するフィールドだけが読まれ、他は無視されます。

| `type`           | 固有フィールド名 | フィールド型                                 |
|------------------|------------------|----------------------------------------------|
| `kustomize`      | `kustomize`      | [ManifestKustomize](./kustomize.md#固有フィールド) |
| `helmfile`       | `helmfile`       | [ManifestHelmfile](./helmfile.md#固有フィールド)   |
| `oras`           | `oras`           | [ManifestORAS](./oras.md#固有フィールド)           |
| `genesissecret`  | `genesisSecret`  | 空オブジェクト（現バージョンではフィールドを持たない） |

`type: genesissecret` は全部小文字ですが、対応するフィールド名は `genesisSecret`（camelCase）です。
YAML キーは camelCase に統一されており、`type` の値だけがプレーンな識別子（全部小文字）になっています。

## type と `path` の対応

`path` は **`tazuna.yaml` 自身の置かれているディレクトリ起点** の相対パスとして解釈されます。
type ごとに何を指すべきかが異なります。

| `type`           | `path` が指す先 |
|------------------|-----------------|
| `kustomize`      | `kustomization.yaml` を含むディレクトリ |
| `helmfile`       | `helmfile.yaml` を含むディレクトリ |
| `oras`           | 実体としては使用されません。バリデーション都合で空にはできないため、適当なディレクトリを書きます。 |
| `genesissecret`  | GenesisSecret YAML **ファイル**（他の type と違い、ディレクトリではなく単一ファイル）|
