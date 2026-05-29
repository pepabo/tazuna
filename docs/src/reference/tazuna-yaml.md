# `tazuna.yaml` スキーマ

このページは Tazuna への唯一の入力ファイルである `tazuna.yaml` の仕様をまとめます。
ここでは Manifest type 別の固有フィールド（`kustomize` / `helmfile` /
`genesissecret` / `oras`）と Test plugin のフィールドには深入りしません。
それらは順次、専用のリファレンスページで扱います。

## ルート (`Tazuna`)

`tazuna.yaml` のルートオブジェクトです。Kubernetes manifest と同じ
`apiVersion` / `kind` / `spec` の 3 つを持ちます。

| フィールド   | 型             | 必須 | デフォルト              | 説明 |
|------------- |----------------|------|--------------------------|------|
| `apiVersion` | string         | -    | -                        | 設定する場合は `tazuna.pepabo.com/v1` と完全一致である必要があります。省略可。 |
| `kind`       | string         | -    | -                        | 設定する場合は `Tazuna` と完全一致である必要があります。省略可。 |
| `spec`       | [TazunaSpec](#tazunaspec) | ◯ | - | Tazuna の振る舞いを定義する本体。 |

最小例:

```yaml
apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
    - name: nginx
      type: kustomize
      path: ./kustomize/nginx
```

## TazunaSpec

| フィールド             | 型                          | 必須 | デフォルト | 説明 |
|------------------------|-----------------------------|------|------------|------|
| `minimumSupportedTazunaVersion` | string             | -    | `""`       | この `tazuna.yaml` を処理するのに必要な tazuna バイナリの最小バージョン（semver）。詳細は [`minimumSupportedTazunaVersion`](#minimumsupportedtazunaversion) 参照。 |
| `manifests`            | [[Manifest](#manifest)]    | ◯    | -          | Tazuna が処理する Manifest の配列。空配列は許容されません。`dependsOn` が使われていれば依存グラフから導出した層順、未使用なら宣言順で実行されます。 |
| `context_matches`      | [string]                    | -    | `[]`       | 現在の kubeconfig context 名がマッチすべき正規表現の配列。空でなければ `apply` / `destroy` 前に評価されます。 |
| `context_match_mode`   | string                      | -    | `or`       | `context_matches` の評価モード。`or`（いずれかに一致）または `and`（すべてに一致）。 |
| `tests`                | [[TestPluginSpec](#tests-フィールド)] | - | `[]` | すべての Manifest 適用後に実行される Test plugin の配列。 |
| `providers`            | [[ProviderConfig](#providers)] | - | `[]`      | GenesisSecret から参照される Secret provider の宣言リスト。組み込みの `default-op` 以外を使う場合に書きます。 |

### `minimumSupportedTazunaVersion`

- この `tazuna.yaml` を安全に処理できる tazuna バイナリの **最小バージョン** を
  semver 形式で宣言します（例: `1.4.0`）。先頭の `v` は許容されます（`v1.4.0` も可）。
- `tazuna.yaml` を読み込む **任意の操作**（`apply` / `destroy` / `plan` / `build` /
  `check` / `status` / `tags` / `state *` など）で、実行中の tazuna のバージョンが
  この値を **下回る** とエラーで終了します。新しい記法を要求する `tazuna.yaml` を
  古いバイナリで誤って処理してしまう事故を防ぎます。
- 未設定（空文字）の場合は制約なしです。
- 値が semver として不正な場合は設定エラーになります。
- 実行中の tazuna がローカルビルド（`dev` など semver でないバージョン）の場合は
  比較をスキップします。ローカル開発がこのゲートでブロックされないようにするためです。

```yaml
spec:
  minimumSupportedTazunaVersion: "1.4.0"
  manifests: []
```

### `context_matches`

- 各要素は Go の `regexp` パッケージでコンパイル可能な正規表現でなければなりません。
  コンパイルに失敗すると `tazuna check` の段階で弾かれます。
- 空配列または未設定の場合、context のチェックは行われません。
- 設定されている場合、`tazuna apply` / `tazuna destroy` はクラスタに触る前に
  current-context を検証します。マッチしないと処理を中断します。

### `context_match_mode`

- `or`（デフォルト）: `context_matches` のいずれか 1 つにでもマッチすれば OK。
- `and`: `context_matches` の **すべて** にマッチする必要があります。
- それ以外の値を指定するとバリデーションエラーになります。

例:

```yaml
spec:
  context_matches:
    - ^staging-
    - -tokyo$
  context_match_mode: and
  manifests: []
```

## Manifest

`spec.manifests[]` の各要素です。1 つの Manifest が、1 つのバックエンド
（kustomize / helmfile / 他）による「クラスタへ入れる単位」に対応します。

| フィールド      | 型                                | 必須 | デフォルト | 説明 |
|-----------------|-----------------------------------|------|------------|------|
| `name`          | string                            | ◯    | -          | Manifest 識別子。`^[a-zA-Z0-9_-]+$` にマッチする必要があり、`includes` 展開後の全 Manifest 間でユニーク。`_metadata` は予約済みで使用不可。 |
| `description`   | string                            | -    | `""`       | 人間向けの説明。挙動には影響しません。 |
| `type`          | string                            | △ (※) | -        | `kustomize` / `helmfile` / `genesissecret` / `oras` のいずれか。 |
| `path`          | string                            | △ (※) | -        | `tazuna.yaml` 自身の置かれているディレクトリ起点の相対パス。 |
| `tags`          | [string]                          | -    | `[]`       | `tazuna apply --tags ...` などで絞り込みに使うタグ。OR 評価。 |
| `dependsOn`     | [string]                          | -    | `[]`       | この Manifest の apply 前に完了している必要がある Manifest 名のリスト。詳細は [`dependsOn`](#dependson) 参照。 |
| `includes`      | [[IncludeFile](#includefile)]     | -    | `[]`       | 別の `tazuna.yaml` を読み込むエントリ。設定時は他の Manifest 固有フィールドは無視されます。詳細は [includes を使う](#includes-を使う) を参照。 |
| `kustomize`     | [ManifestKustomize](#manifest-type-別フィールド) | - | `null` | `type: kustomize` のときに参照されるオプション。 |
| `helmfile`      | [ManifestHelmfile](#manifest-type-別フィールド)  | - | `null` | `type: helmfile` のときに参照されるオプション。 |
| `genesisSecret` | object                            | -    | `null`     | `type: genesissecret` のときに参照されるオプション。現状は空オブジェクト。 |
| `oras`          | [ManifestORAS](#manifest-type-別フィールド)      | - | `null` | `type: oras` のときに参照されるオプション。 |
| `tests`         | [TestPluginSpec]                  | -    | `[]`       | この Manifest の apply 後に実行される Test plugin の配列。 |

(※) `includes` を指定するときは `type` / `path` は不要。
それ以外のときは `type` と `path` が必須です。

### `name`

- 必須。
- 使える文字種は `^[a-zA-Z0-9_-]+$`。
- `_metadata` は内部利用のため予約されており、Manifest 名としては使えません。
- `includes` 展開後の全 Manifest 間で **一意** である必要があります。
  重複していると `tazuna check` でエラーになります。

`tazuna check` では `name` のバリデーションは **エラー** として扱いますが、`tazuna apply`
/ `build` / `destroy` では移行期間として **警告ログのみ** が出る挙動になっています。
新規導入時は `tazuna check` で先に通しておくのが安全です。

### `path`

- `includes` を使わないときは必須。
- **`tazuna.yaml` 自身の置かれているディレクトリ起点** の相対パスとして解釈されます。
  コマンドを実行した cwd 起点ではありません。
- `tazuna check` の時点で実在チェックが行われます。
- type ごとに `path` が指すべき先が異なります。

| `type`          | `path` が指す先 |
|-----------------|-----------------|
| `kustomize`     | `kustomization.yaml` を含むディレクトリ |
| `helmfile`      | `helmfile.yaml` を含むディレクトリ |
| `genesissecret` | GenesisSecret 定義 YAML **ファイル**（ディレクトリではない） |
| `oras`          | 実体としては使用されません。バリデーション都合で空にはできないため、適当なディレクトリを書きます。 |

詳細な解釈は各 [Manifest type 別ページ](./manifest-types/index.md) を参照してください。

### `type`

- `includes` を使わないときは必須。
- 値の一覧は [Manifest type](../concepts/glossary.md#manifest-type) を参照。
- 未対応の値を指定するとバリデーションエラーになります。

### `tags`

- 文字列の配列。Tazuna 自身は内容を解釈しません。
- `--tags` フラグでの絞り込み時に、**指定されたタグのいずれかが付いている** Manifest
  だけが処理対象になります（OR 評価）。

### `dependsOn`

- この Manifest を apply する前に **必ず完了している必要がある** Manifest 名の配列。
- `includes` 展開後の全 Manifest 集合に含まれる名前でなければなりません。
- 自分自身を含めることはできません（自己依存は循環の特殊例として弾かれます）。
- 全体の依存関係に循環が含まれていてはなりません。
- `tazuna.yaml` 内で 1 つでも `dependsOn` が使われていれば Runner は DAG モードに
  切り替わり、同じ依存深度の Manifest を **並列に** 実行します。1 つも使われていなければ
  従来通り宣言順 1 件ずつの実行になります。

詳細と動機は [`dependsOn` による DAG 実行](../concepts/depends-on.md) を参照してください。

例:

```yaml
spec:
  manifests:
    - name: cni
      type: kustomize
      path: ./cni
    - name: cert-manager
      type: helmfile
      path: ./cert-manager
      dependsOn: [cni]
    - name: ingress
      type: helmfile
      path: ./ingress
      dependsOn: [cni]
    - name: app
      type: kustomize
      path: ./app
      dependsOn: [cert-manager, ingress]
```

## Providers

`spec.providers[]` は GenesisSecret から参照される Secret provider の宣言リストです。
組み込みの `default-op`（1Password）以外を使う、または provider を複数並べて使い分けたい
ときに書きます。

```yaml
spec:
  providers:
    - name: primary-op
      type: onepassword
      onepassword: {}
    - name: ops-envfile
      type: envfile
      envfile:
        path: ./secrets/ops.env
```

| フィールド    | 型     | 必須 | デフォルト | 説明 |
|---------------|--------|------|------------|------|
| `name`        | string | ◯    | -          | GenesisSecret の `spec.provider` から参照される名前。`default-op` は予約名で使用不可。 |
| `type`        | string | ◯    | -          | provider 種別。`onepassword` または `envfile`。 |
| `onepassword` | object | △    | `null`     | `type: onepassword` のときに使う追加設定（現状は空オブジェクト）。 |
| `envfile`     | object | △    | `null`     | `type: envfile` のときに使う追加設定。`path` を持つ。 |

詳細は [Secret provider](./secret-providers.md) を参照してください。

## IncludeFile

`manifests[].includes[]` の各要素です。別の `tazuna.yaml` を読み込み、その `manifests[]`
を展開します。

| フィールド | 型     | 必須 | デフォルト | 説明 |
|-----------|--------|------|------------|------|
| `path`    | string | ◯    | -          | 読み込む `tazuna.yaml` のパス。**呼び出し元の `tazuna.yaml` からの相対** で書きます。 |

### includes を使う

```yaml
spec:
  manifests:
    - name: infra
      includes:
        - path: ./infra/tazuna.yaml
        - path: ./addons/tazuna.yaml
```

- `includes` を持つ Manifest は、自身が持つ `type` / `path` / `tags` などの
  「Manifest 本体」のフィールドは **無視** されます。
- `includes` は **ネスト不可** です。include 先の `tazuna.yaml` が
  さらに `includes` を持っていても展開されません。
- include 先で定義された Manifest の `name` も含めて、最終的な全 Manifest 間で
  `name` がユニークである必要があります。

## Manifest type 別フィールド

`type` に対応するフィールド（`kustomize` / `helmfile` / `genesisSecret` /
`oras`）は、それぞれ専用のリファレンスページに切り出しています。

ここでは存在と最低限の役割だけを示します。

| フィールド      | 役割 |
|-----------------|------|
| `kustomize`     | [`type: kustomize`](./manifest-types/kustomize.md) 向けオプション。`defaultNamespace` を持つ。 |
| `helmfile`      | [`type: helmfile`](./manifest-types/helmfile.md) 向けオプション。`vars` / `includeCRDs` / `wait` / `kubeVersion` などを持つ。 |
| `genesisSecret` | [`type: genesissecret`](./manifest-types/genesissecret.md) 向けの拡張点。現バージョンでは空オブジェクト。 |
| `oras`          | [`type: oras`](./manifest-types/oras.md) 向けオプション。`reference` / `delegate` を持つ。 |

## `tests` フィールド

`spec.tests` および `manifests[].tests` の要素である `TestPluginSpec` の詳細仕様は
[Test plugin](./test-plugin.md) を参照してください。
ここでは置かれる位置と実行タイミングだけを示します。

- **全体 `tests`** (`spec.tests`): すべての Manifest 適用が終わったあとに実行されます。
- **個別 `tests`** (`manifests[].tests`): その Manifest の適用直後に実行されます。

## バリデーションのまとめ

`tazuna check` が `tazuna.yaml` に対して行う検証を一覧にしておきます。
**クラスタには触れず**、ここで失敗するものはすべて事前に弾けます。

- `apiVersion` / `kind` を設定するなら値が正規値と完全一致すること。
- `spec.minimumSupportedTazunaVersion` を設定するなら semver として妥当で、実行中の
  tazuna のバージョンがそれ以上であること（ローカルビルドは比較をスキップ）。なお
  この検証は `check` に限らず、`tazuna.yaml` を読み込むすべての操作で実行されます。
- `spec.manifests[]` の各要素について:
  - `includes` が無い場合: `path` と `type` が設定されていること。
  - `type` が既知の値（`kustomize` / `helmfile` / `genesissecret` /
    `oras`）であること。
  - `path` の指す場所が実在すること。
- `spec.manifests[].name` が必須・使用可能文字・ユニーク・予約語禁止を満たすこと。
- `spec.manifests[].dependsOn` が既存 Manifest 名のみを参照し、自己参照・循環依存を含まないこと。
- `spec.context_matches` が正規表現としてコンパイル可能であること。
- `spec.context_match_mode` が `or` / `and` / 未設定のいずれかであること。
- `spec.providers[]` の各要素について: `name` がユニークかつ非空、`default-op` を含まないこと、
  `type` が `onepassword` / `envfile` のいずれかであること、`type` と整合した config を持つこと。
- `type: helmfile` の場合: `helmfile.vars` の各値が `env` / `static` / `op` の
  いずれかを満たすこと（詳細は helmfile のリファレンスページ）。
- `type: oras` の場合: `oras.reference` が必須、`oras.delegate.type` が
  `helmfile` / `kustomize` のいずれかであること。
- `includes` を指定する場合: 各 `include.path` が必須で、ファイルが実在すること。
