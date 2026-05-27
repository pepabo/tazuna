# `type: helmfile`

`helmfile` Manifest は、helmfile で記述された複数の Helm release を、
**`helmfile template` 相当でレンダリングしてからクラスタへ反映** する Manifest type です。

Tazuna は内部で `helmfile/helmfile` パッケージの `app.Template` を呼び、その出力 YAML を
unstructured オブジェクトに変換して `CreateOrUpdate` します。helm の release 履歴は
クラスタ側に保存しません（helm rollback は使えなくなります）。
ブートストラップにおいては rollback よりも宣言的な再生成を優先する、というスタンスです。

## `path`

`helmfile.yaml`（または `helmfile.yaml.gotmpl` などの helmfile が認識するファイル）が
置かれているディレクトリを指します。
**`tazuna.yaml` 自身のディレクトリ起点** の相対パスで書きます。

## 固有フィールド

`manifests[].helmfile` のオブジェクトに書きます。

| フィールド            | 型                                | 必須 | デフォルト | 説明 |
|-----------------------|-----------------------------------|------|------------|------|
| `vars`                | map\<string, [HelmFileVar](#helmfilevar)\> | - | `{}` | helmfile に渡す変数。詳細は [vars](#vars) 参照。 |
| `includeCRDs`         | bool                              | -    | `false`    | helmfile template に `--include-crds` 相当を渡します。 |
| `defaultNamespace`    | string                            | -    | `""`       | レンダリング結果のリソースで `metadata.namespace` が未指定のものに付与する namespace。 |
| `extraValueFiles`     | [string]                          | -    | `[]`       | helmfile template に追加で渡す `--values` ファイル群。 |
| `wait`                | bool                              | -    | `false`    | `true` のとき、`Apply` 後に対象リソースが Ready になるまで待ちます。詳細は [wait の挙動](#wait-の挙動) 参照。 |
| `timeoutSeconds`      | int                               | -    | `0`        | `wait` の最大待機秒数。`0` のときは内部で `300` 秒（5 分）が使われます。 |
| `kubeVersion`         | string                            | -    | `""`       | helmfile template に渡す `--kube-version` の値。 |

## `vars`

`vars` のキーは helmfile 側の変数名、値が [HelmFileVar](#helmfilevar) です。

`tazuna.yaml` のロード時、`vars` は次の順序で解決されます。

1. 各 var の `from` (`env` / `static` / `op`) に応じて値を取得する。
2. 同じディレクトリに [`tazuna.hint.yaml`](../tazuna-hint-yaml.md) があれば、`tazuna.hint.yaml` の
   検証・デフォルト注入を行う。

`vars` に書かないでも `tazuna.hint.yaml` の `default` から値が入る、というケースもあります。
逆に `tazuna.hint.yaml` の制約に違反するとここでエラーになります。

### HelmFileVar

| フィールド     | 型                                | 必須 | 説明 |
|----------------|-----------------------------------|------|------|
| `from`         | string                            | ◯    | 値の取得元。`env` / `static` / `op` のいずれか。 |
| `env`          | string                            | △ (※) | `from: env` のとき必須。参照する環境変数名。 |
| `static`       | string                            | △ (※) | `from: static` のときに使う。スカラー値。 |
| `staticSlice`  | [string]                          | △ (※) | `from: static` のときに使う。スライス値。 |
| `staticMap`    | map\<string, string\>             | △ (※) | `from: static` のときに使う。マップ値。 |
| `op`           | [OnePasswordVaultSelector](#onepasswordvaultselector) | △ (※) | `from: op` のとき必須。 |

(※) `from` の値に応じて、`env` / `static` 系のいずれか 1 つ / `op` が必須です。
`from: static` の場合、`static` / `staticSlice` / `staticMap` のうち **1 つだけ** が設定されている必要があります。

### OnePasswordVaultSelector

| フィールド | 型     | 必須 | 説明 |
|------------|--------|------|------|
| `key`      | string | ◯    | フィールドを `id` で参照するか `label` で参照するか。`id` または `label`。 |
| `vault`    | string | ◯    | 1Password の Vault 名。 |
| `item`     | string | ◯    | 1Password の Item 名。 |
| `field`    | string | ◯    | 取得するフィールド。`key` が `id` ならフィールドの ID、`label` ならラベル。 |

## `wait` の挙動

`wait: true` のとき、`Apply` の終わりに対象リソース全部の Ready 待ちが走ります。
2 秒間隔で polling し、`timeoutSeconds`（未指定なら 300 秒）を超えるとエラー。

各 Kind の Ready 判定:

| Kind          | Ready 条件 |
|---------------|------------|
| `Deployment`  | `spec.replicas == 0` なら即 Ready。それ以外は `status.readyReplicas == status.replicas` かつ `status.availableReplicas == status.replicas` かつ `status.replicas > 0` |
| `StatefulSet` | `spec.replicas == 0` なら即 Ready。それ以外は `status.readyReplicas == status.replicas` かつ `status.replicas > 0` |
| `DaemonSet`   | `status.numberReady == status.desiredNumberScheduled` かつ `status.desiredNumberScheduled > 0` |
| `Pod`         | `status.phase == "Running"` かつ `Ready` condition が `True` |
| その他        | 取得できれば即 Ready 扱い（`ConfigMap` / `Secret` / `Service` など） |

`wait` で待ちきれないリソース固有の条件（CRD の status など）を表現したい場合は、
[Test plugin](../test-plugin.md) の `WaitUntil`（CEL 式）を使うほうが柔軟です。

## 振る舞い

| 操作      | 内部処理 |
|-----------|----------|
| `Build`   | helmfile template の出力 YAML を標準出力に書く。 |
| `Apply`   | helmfile template の結果を unstructured 化し、`defaultNamespace` を補完して順に `CreateOrUpdate`。`wait` が `true` なら Ready 待ち。 |
| `Destroy` | helmfile template の結果を unstructured 化し、`defaultNamespace` を補完して順に削除。`wait` は適用されません。 |

`Apply` / `Destroy` / `Build` のいずれも helmfile template の段階で `vars` を解決します。
解決に失敗した場合（環境変数未設定、1Password の Item にフィールドが無い等）はクラスタには触れずに失敗します。

## 例

```yaml
manifests:
  - name: cert-manager
    type: helmfile
    path: ./helmfile/cert-manager
    helmfile:
      includeCRDs: true
      wait: true
      timeoutSeconds: 120
      vars:
        clusterIssuerEmail:
          from: env
          env: CLUSTER_ISSUER_EMAIL
        dnsProviderApiToken:
          from: op
          op:
            key: label
            vault: Platform
            item: cert-manager
            field: cloudflare-api-token
        extraLabels:
          from: static
          staticMap:
            managed-by: tazuna
            tier: platform
```

## 関連

- helmfile.vars の制約: [`tazuna.hint.yaml` スキーマ](../tazuna-hint-yaml.md)
- 用語: [helmfile](../../concepts/glossary.md#helmfile) /
  [Helm](../../concepts/glossary.md#helm) /
  [1Password](../../concepts/glossary.md#1password)
