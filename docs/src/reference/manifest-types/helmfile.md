# `type: helmfile`

`helmfile` Manifest は、[helmfile](https://github.com/helmfile/helmfile) 形式 (のサブセット)
で記述された複数の Helm release を、**`helmfile template` 相当でレンダリングしてから
クラスタへ反映** する Manifest type です。

> **互換性について**
> Tazuna は helmfile 本体には依存しません。`helmfile.yaml` の **サブセット** を独自に解釈し、
> 内部的に Helm パッケージ (`helm.sh/helm/v3`) の **in-memory render**
> (`action.Install{ClientOnly, DryRun}`) でマニフェストを生成します。形式・着想は helmfile
> から得ていますが、helmfile そのものではありません。サポートするフィールドは
> [対応する helmfile サブセット](#対応する-helmfile-サブセット) を参照してください。

レンダリング結果の YAML は unstructured オブジェクトに変換し、Server-Side Apply
(FieldOwner=`tazuna`) でクラスタへ適用します。helm の release 履歴はクラスタ側に
保存しません（helm rollback は使えません）。ブートストラップにおいては rollback よりも
宣言的な再生成を優先する、というスタンスです。

> 以前の実装は helmfile 本体の `app.Template` を呼び、その標準出力をグローバルに
> `os.Stdout` を差し替えてキャプチャしていました。これは並列 apply 時にレンダリング結果が
> 混線し得るバグの温床でしたが、in-memory render への移行により根治しています。

## `path`

`helmfile.yaml`（または `helmfile.yaml.gotmpl`）ファイル、もしくはそれらが置かれている
**ディレクトリ** を指します。ディレクトリを指定した場合は
`helmfile.yaml.gotmpl` → `helmfile.yaml` → `helmfile.yml.gotmpl` → `helmfile.yml`
の順で探索します。
**`tazuna.yaml` 自身のディレクトリ起点** の相対パスで書きます。

## 対応する helmfile サブセット

以下の `helmfile.yaml` の構造を解釈します。

```yaml
repositories:                   # 省略可。chart を <alias>/<chart> 形式で参照する場合に宣言する
  - name: <alias>
    url: <リポジトリ URL>        # https://... もしくは oci://...
    # username: <basic 認証ユーザ>  # 省略可 (HTTP(S) リポジトリのみ)
    # password: <basic 認証パスワード>
    # oci: true                    # url が oci:// でない registry を OCI 扱いにする場合

releases:
  - name: <release 名>
    namespace: <namespace>      # 省略時は defaultNamespace で補完
    chart: <chart 参照>          # 下記のいずれか
    version: <バージョン>        # リモート chart では必須、ローカルチャートでは情報用
    values:
      - <value ファイルへの相対パス>
      - <インラインの値 (map)>
```

- `helmfile.yaml` 自体を Go テンプレートとして評価します。`.StateValues.<name>` /
  `.Values.<name>` で [`vars`](#vars) を参照でき、`default` などの
  [sprig](https://masterminds.github.io/sprig/) 関数が使えます。
  ただし `env` / `expandenv` は使えません (ORAS 経由で取得したリモートの
  helmfile が実行者の環境変数を読み取るのを防ぐため)。環境変数は
  [`vars`](#vars) の `from: env` で明示的に渡してください。
- `chart` は次の 3 形式に対応します。
  - **ローカルチャートへの相対パス** (例: `./mychart`、`charts/mychart`)。
    相対パスは `helmfile.yaml` のあるディレクトリ起点で解決します。
  - **OCI チャート参照** (例: `oci://public.ecr.aws/karpenter/karpenter-crd`)。
    `version` を指定し、helm registry client 経由で pull します。
  - **リポジトリ alias 形式** (`<alias>/<chart>`、例: `argo-cd/argo-cd`)。
    `<alias>` が `repositories:` で宣言された名前と一致する場合、その `url`
    (HTTP(S) or OCI) から `version` の chart を pull します
    (`helm repo add` は不要)。`<alias>` が未宣言の場合は従来どおりローカル
    相対パスとして解決します。
- `values` は value ファイルのパスとインライン map を順にマージし、最後に
  [`extraValueFiles`](#固有フィールド) を上書きとしてマージします。

helmfile 本体の以下の機能は **未対応** です: environments /
`bases` / release 間の `needs` / `hooks` / `--selector` 等。これらが必要な場合は
helmfile でレンダリングした結果を [`type: kustomize`](./kustomize.md) などで取り込んでください。
ただしテンプレート内の `{{ .Environment.Name }}` は参照でき、tazuna の
`-e/--environment` フラグの値が注入されます (未指定時は `"default"`)。

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
| `Build`   | helm の in-memory render の結果 YAML を返す。 |
| `Apply`   | render 結果を unstructured 化し、`defaultNamespace` を補完して順に Server-Side Apply。`wait` が `true` なら Ready 待ち。 |
| `Destroy` | render 結果を unstructured 化し、`defaultNamespace` を補完して順に削除。`wait` は適用されません。 |

`Apply` / `Destroy` / `Build` のいずれも render の段階で `vars` を解決します。
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
