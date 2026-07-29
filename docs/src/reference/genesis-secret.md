# GenesisSecret スキーマ

GenesisSecret は、外部の Secret ストア（現バージョンでは 1Password）にある秘匿情報を取得し、
Kubernetes Secret として **生成** するための宣言です。

GenesisSecret は Kubernetes の CRD ではなく、**Tazuna が読む YAML スキーマ** です。
クラスタに `GenesisSecret` リソースが現れるわけではなく、適用結果として **`Secret`** が現れます。

`tazuna.yaml` からは `type: genesissecret` の Manifest として参照します。

```yaml
# tazuna.yaml
spec:
  manifests:
    - name: cloud-credentials
      type: genesissecret
      path: ./genesissecrets/cloud.yaml
```

`type: genesissecret` の `path` は **YAML ファイル 1 つを直接指します**
（他の Manifest type のようにディレクトリを指すのではありません）。

## ルート (`GenesisSecret`)

| フィールド   | 型                                    | 必須 | デフォルト | 説明 |
|--------------|---------------------------------------|------|------------|------|
| `apiVersion` | string                                | -    | -          | スキーマバージョンを示します。値は現状検証されません。 |
| `kind`       | string                                | -    | -          | リソース種別を示します。値は現状検証されません。 |
| `spec`       | [GenesisSecretSpec](#genesissecretspec) | ◯  | -          | GenesisSecret 本体。 |

`apiVersion` / `kind` のフィールドは構造体に対応する宣言がなく、書いても
読まれずに無視されますが、慣習として `apiVersion: tazuna.pepabo.com/v1` /
`kind: GenesisSecret` を書いておくと、後から検証が入っても揃えやすくなります。

## GenesisSecretSpec

| フィールド  | 型                                              | 必須 | デフォルト | 説明 |
|-------------|-------------------------------------------------|------|------------|------|
| `provider`  | string                                          | -    | `""`       | 取得元 Provider の名前。`tazuna.yaml` の [`spec.providers[]`](./tazuna-yaml.md#providers) に宣言された `name` のいずれか、または組み込みの `default-op` を指定します。空文字のときは後方互換のため `default-op` にフォールバックします。詳細は [Secret provider](./secret-providers.md) を参照してください。 |
| `secrets`   | [[GenesisSecretGenerate](#genesissecretgenerate)] | ◯ | -        | 取得対象。複数書けます。 |
| `outputs`   | [[GenesisSecretOutput](#genesissecretoutput)]     | ◯ | -        | 出力先。複数書けます。 |

## GenesisSecretGenerate

`secrets[]` の各要素です。1 つの「Provider 上のアイテム」を表します。

| フィールド     | 型                                          | 必須 | デフォルト | 説明 |
|----------------|---------------------------------------------|------|------------|------|
| `uri`          | string                                      | ◯    | -          | Provider 上のアイテムを指す URI。詳細は [`uri` の形式](#uri-の形式) 参照。 |
| `items`        | map\<string, [GenesisSecretGenerateItem](#genesissecretgenerateitem)\> | ◯ | - | Provider から取得した key と、出力 Secret 上のキー名の対応表。 |
| `preferLabel`  | bool                                        | -    | `false`    | Provider が返したフィールドを **ラベル名** でキー化するかどうか。`false` のときは ID（ランダム文字列になる場合がある）でキー化されます。1Password で人間が付けたフィールド名を `items` のキーに書きたい場合は `true` にします。 |

### `uri` の形式

1Password Provider では、`url.Parse` の結果のうち **path の 1 つ目を vault 名、2 つ目を item 名**
として解釈します。scheme やホストは現バージョンでは使われません。

`tazuna secret-to-genesissecret` が自動生成するときは次の形式で書き出します。

```text
op://<op-host>/<vault>/<item>
```

例:

```yaml
uri: op://example.1password.com/example-vault/cloud-credentials
```

scheme やホストはパースには通りますが、参照されません。
将来別 Provider を増やしたときに使い分けるためのスペースとして残されている、と理解しておくと安全です。

### GenesisSecretGenerateItem

`items` マップの **値** にあたる構造です（キーは Provider から返ってきた field の ID または label）。

| フィールド | 型     | 必須 | デフォルト | 説明 |
|-----------|--------|------|------------|------|
| `mapTo`   | string | ◯    | -          | 出力先 Kubernetes Secret の `data` キー名。Provider から取得した値はこのキー名で Secret に格納されます。 |

例:

```yaml
items:
  accessKeyID:
    mapTo: AWS_ACCESS_KEY_ID
  secretAccessKey:
    mapTo: AWS_SECRET_ACCESS_KEY
```

`items` のキー `accessKeyID` が Provider 上のフィールド名（`preferLabel: true` ならラベル名）に対応し、
`mapTo` がそのまま Kubernetes Secret のキー名になります。
`items` のキーが Provider 側に存在しないとエラーになります。

## GenesisSecretOutput

`outputs[]` の各要素です。1 つの「出力先」を表します。

| フィールド          | 型                                                                          | 必須 | デフォルト | 説明 |
|---------------------|-----------------------------------------------------------------------------|------|------------|------|
| `kubernetesSecret`  | [GenesisSecretOutputKubernetesSecret](#genesissecretoutputkubernetessecret) | △ (※) | `null`   | 出力先として Kubernetes Secret を作る場合に指定します。 |
| `stdout`            | object                                                                      | △ (※) | `null`   | 取得した値を標準出力に dotenv 形式（`KEY=VALUE` 1 行 1 ペア、ソート済み）で書き出す場合に指定します。現状フィールドは空オブジェクト `{}` で構いません。 |

(※) `outputs[]` の各要素は `kubernetesSecret` か `stdout` の **どちらか一方** を
指定する必要があります。両方を同時に指定したり、両方とも `null` の場合は
バリデーションエラーになります。

### `stdout`

`stdout: {}` を指定した output は、Provider から取得した値を **dotenv 形式** で
標準出力に書き出します。1Password で運用していた値を envfile に移行する作業や、
シェル `eval` で環境変数として読み込みたいケースで使えます。

```yaml
outputs:
  - stdout: {}
```

出力フォーマット:

```text
AWS_ACCESS_KEY_ID=AKIA...
AWS_SECRET_ACCESS_KEY=...
```

行の並びは **キー名の昇順** で安定しています。
クラスタには触らないため、`kubectl` の権限を持たないオペレーターでも実行できます。

### GenesisSecretOutputKubernetesSecret

| フィールド      | 型                | 必須 | デフォルト                | 説明 |
|-----------------|-------------------|------|---------------------------|------|
| `namespace`     | string            | ◯    | -                         | 出力する Secret の namespace。 |
| `name`          | string            | ◯    | -                         | 出力する Secret の name。 |
| `labels`        | map\<string, string\> | -    | `null`                | 出力する Secret に付ける labels。 |
| `annotations`   | map\<string, string\> | -    | `null`                | 出力する Secret に付ける annotations。 |
| `type`          | string            | -    | `Opaque`                  | corev1 の SecretType。空文字列のときは `Opaque`（厳密には `kubernetes.io/opaque` ではなく Kubernetes のデフォルト `Opaque`）として扱われます。`kubernetes.io/tls` などを指定できます。 |
| `context`       | string            | -    | `""`                      | 構造体上は存在しますが、**現バージョンの Manager 実装では参照されません。** 出力先クラスタは Tazuna 全体の current-context が使われます。 |

## 解決の流れ

`tazuna apply` 時、`type: genesissecret` の Manifest は次のように処理されます。

1. `manifests[].path` の指す **YAML ファイル**（`tazuna.yaml` 自身のディレクトリ起点）を読む。
2. `spec.secrets[]` の各要素を Provider に渡し、フィールド集合を取得する。
3. `items` の `mapTo` でキー名をリネームしながら、すべての `secrets[]` の結果を 1 つの
   `map[string]string` にマージする（同じキーが衝突した場合は **後勝ち**）。
4. `spec.outputs[]` の各 `kubernetesSecret` について、`namespace` / `name` を持つ
   Kubernetes `Secret` を `CreateOrUpdate` する。
   - `StringData` にマージ済みの map がそのまま入る。
   - `labels` / `annotations` / `type` は宣言どおりに付与される。

`tazuna destroy` 時も同じ Provider 取得が走り、`outputs[].kubernetesSecret` の
`namespace` / `name` で示される `Secret` を削除します。

`tazuna build` 時は、出力対象が `outputs[0].kubernetesSecret` 1 件分の Secret YAML として
標準出力に書き出されます（複数 `outputs` を書いていても、build では先頭 1 件のみが対象）。

## State と always-sync

GenesisSecret から生成される Secret は、`tazuna state diff` 上で常に
`always-sync` 分類になります。
ContentHash で差分判定できる対象ではなく、Provider 側を真実の源として
毎回同期する扱いです。詳細は [Diff type](../concepts/glossary.md#diff-type) /
[always-sync](../concepts/glossary.md#always-sync) を参照してください。

## 例

最小例:

```yaml
apiVersion: tazuna.pepabo.com/v1
kind: GenesisSecret
spec:
  secrets:
    - uri: op://example.1password.com/example-vault/cloud-credentials
      preferLabel: true
      items:
        accessKeyID:
          mapTo: AWS_ACCESS_KEY_ID
        secretAccessKey:
          mapTo: AWS_SECRET_ACCESS_KEY
  outputs:
    - kubernetesSecret:
        namespace: default
        name: cloud-credentials
```

`type: kubernetes.io/tls` を出力する例:

```yaml
apiVersion: tazuna.pepabo.com/v1
kind: GenesisSecret
spec:
  secrets:
    - uri: op://example.1password.com/example-vault/tls-certificate
      preferLabel: true
      items:
        certificate:
          mapTo: tls.crt
        privateKey:
          mapTo: tls.key
  outputs:
    - kubernetesSecret:
        namespace: ingress-nginx
        name: example-tls
        type: kubernetes.io/tls
        labels:
          managed-by: tazuna
```

## 関連

- `tazuna.yaml` 側からの参照: [`tazuna.yaml` Manifest type 別フィールド](./tazuna-yaml.md#manifest-type-別フィールド)
- Provider の語彙: [Provider (SecretProvider)](../concepts/glossary.md#provider-secretprovider)
- 既存 Secret を 1Password と GenesisSecret に書き出す:
  [`tazuna secret-to-genesissecret`](./cli/secret-to-genesissecret.md)
- 用語: [GenesisSecret](../concepts/glossary.md#genesissecret)
