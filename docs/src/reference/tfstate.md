# tfstate 参照

Terraform で作ったリソースの値（VPC ID、IAM Role ARN、DB のエンドポイントなど）を、
`tazuna.yaml` から **tfstate を直接読んで** 参照できます。値を手で書き写したり、
環境変数でいちいち受け渡したりする必要はありません。

このページでは `spec.tfstates[]` の宣言方法、backend ごとの設定、アドレスの書式、
値を参照する 3 つの方法についてまとめます。手順を追って試したいときは
[Terraform の値を使う](../guides/terraform-values.md) を参照してください。

## 全体像

```yaml
spec:
  # 1. 参照先の tfstate に名前を付けて宣言する
  tfstates:
    - name: network
      backend:
        type: s3
        s3:
          bucket: example-tfstate
          key: network/terraform.tfstate
          region: ap-northeast-1

  # 3. GenesisSecret から参照する場合は tfstate 型の provider を宣言する
  providers:
    - name: tf-network
      type: tfstate
      tfstate:
        source: network

  manifests:
    - name: app
      type: helmfile
      path: ./helmfile/app
      helmfile:
        vars:
          # 2. helmfile vars から参照する
          vpcId:
            from: tfstate
            tfstate:
              source: network
              address: output.vpc_id
      # 4. tazuna.yaml のどこからでも template 関数で参照する
      tags: [{{ tfstate "network" "output.environment" }}]
```

値を参照する方法は次の 3 つです。どれも `spec.tfstates[].name` でソースを指定します。

| 方法 | 書く場所 | 値の型 | 用途 |
|------|----------|--------|------|
| [helmfile vars `from: tfstate`](#helmfile-vars-から参照する) | `manifests[].helmfile.vars` | tfstate の型のまま（string / number / bool / list / map） | Helm chart に値を渡す |
| [template 関数 `tfstate` / `tfstateJSON`](#template-関数から参照する) | `tazuna.yaml` と `includes` で読むファイルの全体 | 文字列 | `defaultNamespace` や `tags` など、任意のフィールド |
| [Secret provider `type: tfstate`](#genesissecret-から参照する) | GenesisSecret YAML | 文字列 | tfstate 内の値（sensitive output など）を Kubernetes Secret にする |

## `spec.tfstates[]`

| フィールド | 型 | 必須 | 説明 |
|------------|----|------|------|
| `name`     | string | ◯ | ソースの名前。`spec.tfstates[]` の中で一意である必要があります。 |
| `backend`  | [TFStateBackend](#tfstatebackend) | ◯ | tfstate を読み出す backend の設定。 |

### TFStateBackend

| フィールド | 型 | 必須 | 説明 |
|------------|----|------|------|
| `type`  | string | ◯ | backend の種類。`s3` または `local`。 |
| `s3`    | [S3 backend](#type-s3) | △ | `type: s3` のときに必須。 |
| `local` | [local backend](#type-local) | △ | `type: local` のときに必須。 |

`type` と違う backend の設定（例: `type: s3` なのに `local:` も書いてある）は
バリデーションエラーになります。

### `type: s3`

terraform の [`backend "s3"`](https://developer.hashicorp.com/terraform/language/backend/s3)
と同じ規則で tfstate オブジェクトを特定します。S3 互換ストレージにも対応しています。

| フィールド | 型 | 必須 | デフォルト | 説明 |
|------------|----|------|------------|------|
| `bucket`             | string | ◯ | - | tfstate が置かれているバケット。 |
| `key`                | string | ◯ | - | tfstate オブジェクトのキー。terraform の backend 設定と同じく、workspace を含まないキーを書きます。 |
| `region`             | string | - | AWS SDK の標準解決 | バケットのリージョン。省略すると `AWS_REGION` / `AWS_DEFAULT_REGION` / profile の `region` の順で解決します。 |
| `profile`            | string | - | `AWS_PROFILE`、なければ `default` | 使用する AWS shared config の profile。 |
| `workspace`          | string | - | `""` | terraform workspace 名。空または `default` のときは `key` をそのまま読み、それ以外のときは `<workspaceKeyPrefix>/<workspace>/<key>` を読みます。 |
| `workspaceKeyPrefix` | string | - | `env:` | workspace を使うときのキーの prefix。terraform の `workspace_key_prefix` に対応します。 |
| `endpoint`           | string | - | `""` | S3 API のエンドポイント URL を上書きします（MinIO などの S3 互換ストレージ向け）。 |
| `usePathStyle`       | bool   | - | `false` | path-style のアドレス指定（`https://<endpoint>/<bucket>/<key>`）を使います。 |

**認証情報は `tazuna.yaml` に書きません。** AWS SDK 標準の credential chain
（環境変数、`AWS_PROFILE`、SSO、`credential_process`、ECS / EC2 のインスタンスロールなど）
で解決します。CI では OIDC で AssumeRole したうえで `AWS_*` 環境変数を渡すのが基本です。

必要な IAM 権限は次のとおりです。

- 対象オブジェクトへの `s3:GetObject`
- バケットが SSE-KMS で暗号化されている場合は、その KMS キーへの `kms:Decrypt`

tazuna は tfstate を **読むだけ** です。書き込みもロック（DynamoDB / S3 lockfile）もしません。

workspace の例を示します。terraform で `workspace_key_prefix` を変えていなければ、
`workspace: prod` と書くと `env:/prod/network/terraform.tfstate` を読みます。

```yaml
tfstates:
  - name: network-prod
    backend:
      type: s3
      s3:
        bucket: example-tfstate
        key: network/terraform.tfstate
        workspace: prod
        region: ap-northeast-1
```

### `type: local`

ローカルファイルの tfstate を読みます。terraform の `backend "local"`（または backend
未指定のときの `terraform.tfstate`）に対応します。

| フィールド | 型 | 必須 | 説明 |
|------------|----|------|------|
| `path` | string | ◯ | tfstate ファイルのパス。相対パスは `tazuna.yaml` のディレクトリから解決します。 |

local backend の workspace は `terraform.tfstate.d/<workspace>/terraform.tfstate` に
保存されるので、`path` にそのパスを書きます。

### 環境ごとに切り替える

`spec.tfstates[]` も [テンプレート変数](./tazuna-yaml.md#テンプレート変数) で描画されるので、
`-e/--environment` に合わせて参照先を変えられます。

```yaml
tfstates:
  - name: network
    backend:
      type: s3
      s3:
        bucket: example-tfstate
        key: network/terraform.tfstate
        workspace: {{ .Environment }}
        region: ap-northeast-1
```

ただし、`spec.tfstates[]` の中で `tfstate` / `tfstateJSON` 関数を使うことはできません
（[読み込みの仕組み](#読み込みの仕組み) を参照）。

## アドレスの書式

tfstate 内の値は **アドレス** で指定します。書式は terraform の
resource address と、値の中をたどるパスを組み合わせたものです。

```text
output.<name>[<path>]
[module.<name>[<index>].]...[data.]<type>.<name>[<index>].<attribute>[<path>]
```

`<path>` は `.key`、`[0]`、`["key"]` を繰り返して、値の内側をたどります。
list の要素は `.0` のように書くこともできます。

| 例 | 指すもの |
|----|----------|
| `output.vpc_id` | output `vpc_id` の値 |
| `output.private_subnet_ids[0]` | list 型 output の先頭要素 |
| `output.cluster.endpoint` / `output.cluster["endpoint"]` | object 型 output の要素 |
| `aws_vpc.main.id` | resource `aws_vpc.main` の attribute `id` |
| `aws_subnet.private[1].id` | `count` を使った resource の 2 番目のインスタンス |
| `aws_iam_role.this["web"].arn` | `for_each` を使った resource のインスタンス |
| `data.aws_caller_identity.current.account_id` | data source の attribute |
| `module.network.aws_vpc.this.id` | module 内の resource |
| `module.app["web"].module.db.aws_db_instance.this[0].endpoint` | ネストした module（`for_each` / `count` 付き）内の resource |
| `aws_security_group.app.tags.Name` | map 型 attribute の要素 |

補足:

- `count` / `for_each` を使っている resource では、インスタンスの添字（`[0]` / `["key"]`）が必須です。
  省略するとエラーになり、メッセージに使える添字の一覧が表示されます。
- resource の attribute 名は、terraform の provider が state に書く名前そのままです。
  `terraform state show <address>` で確認できます。
- `terraform_data` の `input` / `output` のような dynamic 型の attribute は、state に
  `{"value": ..., "type": ...}` という形で保存されています。この場合は
  `terraform_data.x.output.value` のように `.value` を付けてください。
- output は sensitive であっても読めます（tfstate には平文で保存されているため）。
  sensitive な値は helmfile vars や template 関数ではなく、GenesisSecret から参照することを
  おすすめします。
- 対応している tfstate の format version は `4`（Terraform 0.12 以降と OpenTofu）です。

見つからないアドレスを指定すると、output 名、resource アドレス、map のキー、
インスタンスの添字といった **候補** を含むエラーになります（値そのものは表示されません）。

```text
tfstate source "network": failed to look up "output.vpcid":
  output "vpcid" not found in tfstate (available outputs: private_subnet_ids, vpc_id)
```

## helmfile vars から参照する

[`helmfile.vars`](./manifest-types/helmfile.md#vars) で `from: tfstate` を指定します。

```yaml
manifests:
  - name: aws-load-balancer-controller
    type: helmfile
    path: ./helmfile/aws-load-balancer-controller
    helmfile:
      vars:
        vpcId:
          from: tfstate
          tfstate:
            source: network
            address: output.vpc_id
        subnetIds:
          from: tfstate
          tfstate:
            source: network
            address: output.private_subnet_ids   # list のまま渡される
        roleArn:
          from: tfstate
          tfstate:
            source: iam
            address: module.irsa["alb"].aws_iam_role.this.arn
```

| フィールド | 型 | 必須 | 説明 |
|------------|----|------|------|
| `tfstate.source`  | string | ◯ | `spec.tfstates[].name` のいずれか。 |
| `tfstate.address` | string | ◯ | [アドレス](#アドレスの書式)。 |

値は **tfstate の型のまま** helmfile に渡されます。string / number / bool / list / map を
`.StateValues.<name>` からそのまま使えます（list は `toYaml` で values に展開できます）。
値が `null` の場合はエラーになります。

## template 関数から参照する

`tazuna.yaml` 本体と、`includes` で読み込むファイルでは、次の 2 つの template 関数を使えます。

| 関数 | 戻り値 |
|------|--------|
| `{{ tfstate "<source>" "<address>" }}` | 値を文字列にしたもの。string はそのまま、number / bool は文字列化、list / map は JSON。`null` はエラー。 |
| `{{ tfstateJSON "<source>" "<address>" }}` | 値の JSON 表現。YAML の flow 形式としてそのまま埋め込めます（string は `"..."` で囲まれます）。 |

```yaml
manifests:
  - name: app
    type: kustomize
    path: ./kustomize/app
    kustomize:
      defaultNamespace: {{ tfstate "platform" "output.app_namespace" }}
    tags: {{ tfstateJSON "platform" "output.app_tags" }}   # 例: ["web","batch"]
```

template 関数を使うと、`tazuna.yaml` を読み込むすべてのコマンド（`check` / `tags` /
`state list` などを含む）で backend へのアクセスが発生します。オフラインで `check` を
実行したい CI ジョブでは、helmfile vars か GenesisSecret から参照するようにしてください。
こちらは manifest を render するときにだけ読み込まれます。

## GenesisSecret から参照する

`spec.providers[]` に `type: tfstate` の provider を宣言し、GenesisSecret の
`spec.provider` にその名前を指定します。`secrets[].items` の **キーをアドレスとして** 解釈し、
`mapTo` に指定した Secret のキーへ値を入れます。

```yaml
# tazuna.yaml
spec:
  tfstates:
    - name: database
      backend:
        type: s3
        s3:
          bucket: example-tfstate
          key: database/terraform.tfstate
          region: ap-northeast-1
  providers:
    - name: tf-database
      type: tfstate
      tfstate:
        source: database
```

```yaml
# GenesisSecret YAML
apiVersion: tazuna.pepabo.com/v1
kind: GenesisSecret
spec:
  provider: tf-database
  secrets:
    - items:
        output.master_password:
          mapTo: DB_PASSWORD
        aws_db_instance.main.address:
          mapTo: DB_HOST
        aws_db_instance.main.port:
          mapTo: DB_PORT        # number は "5432" のように文字列化される
  outputs:
    - kubernetesSecret:
        namespace: app
        name: database
```

- `secrets[].uri` と `preferLabel` は使いません。
- 値は template 関数 `tfstate` と同じ規則で文字列になります。`null` はエラーです。

provider の宣言方法については [Secret provider](./secret-providers.md#type-tfstate) も参照してください。

## 読み込みの仕組み

### 2 パスで描画する

`spec.tfstates[]` は `tazuna.yaml` の中に書かれているので、tazuna は `tazuna.yaml` を
次の 2 パスで描画します。

1. `tfstate` / `tfstateJSON` を **stub**（空の値を返し、backend にアクセスしない）にして描画し、
   `spec.tfstates[]` を読み取る。
2. 本物の関数を使ってもう一度描画する。

このため、`spec.tfstates[]` 自体（と `minimumSupportedTazunaVersion`）を tfstate の値に
依存させることはできません。`{{ .Environment }}` は使えます。

### 遅延読み込みとキャッシュ

- tfstate は、そのソースが **初めて参照されたときにだけ** backend から読み込まれます。
  宣言しただけで参照していないソースにはアクセスしません。
- 1 回の実行の中では、ソースごとに 1 回だけ読み込みます。`tazuna.yaml` の描画、include、
  helmfile vars、GenesisSecret で同じソースを参照しても、backend へのアクセスは 1 回です。
  `dependsOn` による並列 apply でも同じです。
- 読み込みに失敗した場合も、その実行の中では結果（エラー）を使い回します。

### バリデーション（`tazuna check`）

`tazuna check` では、backend にアクセスせずに次の点を検査します。

- `spec.tfstates[].name` が空でないこと、重複していないこと
- `backend.type` が対応している種類であること、`type` に対応する設定が揃っていること
  （`s3.bucket` / `s3.key`、`local.path`）
- helmfile vars の `tfstate.source` / `tfstate.address` が揃っていて、アドレスの書式が正しいこと
- helmfile vars と `type: tfstate` の provider が参照する `source` が `spec.tfstates[]` に宣言されていること

アドレスが tfstate の中に実際に存在するかどうかは、値を読み込むとき（template 関数なら
`tazuna.yaml` を読み込んだ直後、helmfile vars と GenesisSecret なら render するとき）に検査されます。

## ログとトレース

`--log-level debug` を付けると、次のログが出力されます。

```text
level=DEBUG msg="tfstate: loading state" source=network backend=s3 bucket=example-tfstate key=network/terraform.tfstate region=ap-northeast-1
level=DEBUG msg="tfstate: loaded state" source=network backend=s3 ... bytes=61158 serial=13 terraform_version=1.10.5 outputs=4 resources=30 elapsed=182ms
level=DEBUG msg="tfstate: resolved value" source=network address=output.vpc_id value_kind=string
```

**tfstate の値はログに一切出力しません**（出すのは値の種類 `value_kind` だけです）。
エラーメッセージにも値は含めません。

OpenTelemetry のトレース（`--otlp-endpoint`）では、ソースごとに `TFState.Load` span が
記録されます。attribute は `tfstate.source` / `tfstate.backend` / `tfstate.serial` /
`tfstate.terraform_version` です。

## 新しい backend を追加する（コントリビュータ向け）

backend は `pkg/tfstate` の `Backend` interface で抽象化されています。

```go
type Backend interface {
    Read(ctx context.Context) ([]byte, error) // tfstate の JSON をそのまま返す
    LogAttrs() []slog.Attr                    // ログに出す「どこを読んでいるか」（認証情報や値は含めない）
}

type BackendFactory func(cfg v1.TFStateBackend, baseDir string) (Backend, error)
```

gcs や azurerm などに対応するときは、次の手順で追加します。

1. `api/v1/tfstate_types.go` に `TFStateBackendType` の定数と、設定用の構造体
   （`TFStateBackend` のフィールド）を追加する。
2. `pkg/tfstate` に `Backend` の実装と `BackendFactory` を追加し、`DefaultFactories()` に登録する。
   Factory の中では設定値の検査だけを行い、ネットワークアクセスや認証情報の読み込みは
   `Read` まで遅らせる（`tazuna check` で backend にアクセスしないようにするため）。
3. `pkg/validator/tfstate.go` の `validateTFStateBackendExclusive` に新しい設定フィールドを追加する。
4. このページの backend 一覧を更新する。

アドレスの解決、キャッシュ、ログ、3 つの参照方法は backend に依存しないので、
変更する必要はありません。

## 関連

- ガイド: [Terraform の値を使う](../guides/terraform-values.md)
- [`tazuna.yaml` スキーマ](./tazuna-yaml.md)
- [`type: helmfile`](./manifest-types/helmfile.md)
- [Secret provider](./secret-providers.md)
