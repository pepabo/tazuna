# `type: genesissecret`

`genesissecret` Manifest は、別ファイルに書いた **[GenesisSecret YAML](../genesis-secret.md)**
を読み込み、外部 Secret ストア（現バージョンでは 1Password）から取得した値で
Kubernetes Secret を生成する Manifest type です。

`tazuna.yaml` 側でこの Manifest type が担うのは「**どの GenesisSecret YAML を読むか**」だけです。
中の `spec.secrets` / `spec.outputs` などの仕様は [GenesisSecret スキーマ](../genesis-secret.md)
を参照してください。

## `path`

他の Manifest type と違い、`path` は **ディレクトリではなく YAML ファイル 1 つを直接指します**。
**`tazuna.yaml` 自身のディレクトリ起点** の相対パスで書きます。

```yaml
manifests:
  - name: aws-credentials
    type: genesissecret
    path: ./genesissecrets/aws.yaml   # ← ファイルを直接指す
```

## 固有フィールド

`manifests[].genesisSecret` のオブジェクトに書きます。

現バージョンでは **フィールドを持たない空オブジェクト** です。
将来の拡張のために予約されているフィールド名です。

```yaml
manifests:
  - name: aws-credentials
    type: genesissecret
    path: ./genesissecrets/aws.yaml
    # genesisSecret: {}  # 現状は中身が空のため書く必要なし
```

## 振る舞い

| 操作      | 内部処理 |
|-----------|----------|
| `Build`   | GenesisSecret YAML を読み込み、Provider から値を取得し、`outputs[0].kubernetesSecret` 1 件分の Secret YAML を標準出力に書く。 |
| `Apply`   | GenesisSecret YAML を読み込み、Provider から値を取得し、`outputs[].kubernetesSecret` の各エントリに対して Kubernetes Secret を `CreateOrUpdate` する。 |
| `Destroy` | GenesisSecret YAML を読み込み（Provider 取得も実行されます）、`outputs[].kubernetesSecret` の `namespace` / `name` に該当する Secret を削除する。 |

`Build` は `outputs` の先頭 1 件のみを出力する点が `Apply` と異なります（複数 outputs を
書いていても、`tazuna build` の出力は 1 件分です）。詳細は
[GenesisSecret - 解決の流れ](../genesis-secret.md#解決の流れ) を参照してください。

## State との関係

`type: genesissecret` から生成される Secret は、`tazuna state diff` 上で常に `always-sync`
として扱われます。ContentHash で差分判定する対象ではなく、Provider 側を真実の源として
毎回同期されます。詳細は [State の内部構造 - Diff type](../state.md#diff-type) と
[GenesisSecret - State と always-sync](../genesis-secret.md#state-と-always-sync) を参照してください。

## 関連

- GenesisSecret YAML のスキーマ: [GenesisSecret スキーマ](../genesis-secret.md)
- 既存 Secret から GenesisSecret を書き出す: [`tazuna secret-to-genesissecret`](../cli/secret-to-genesissecret.md)
- 用語: [GenesisSecret](../../concepts/glossary.md#genesissecret) /
  [Provider (SecretProvider)](../../concepts/glossary.md#provider-secretprovider) /
  [always-sync](../../concepts/glossary.md#always-sync)
