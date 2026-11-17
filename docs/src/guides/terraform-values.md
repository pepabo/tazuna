# Terraform の値を使う

クラスタの bootstrap では、Terraform で作ったリソースの値がよく必要になります。
たとえば aws-load-balancer-controller に渡す VPC ID、IRSA 用の IAM Role ARN、
アプリに渡す DB のエンドポイントやパスワードなどです。

このガイドでは、S3 に置かれた tfstate から値を読み、helmfile と GenesisSecret に渡すまでを
順に進めます。仕様の詳細は [tfstate 参照](../reference/tfstate.md) を参照してください。

## 前提

- Terraform の state が S3 backend に保存されていること
- tazuna を実行する環境で、その S3 オブジェクトを読める AWS 認証情報が使えること
  （`aws s3 cp s3://<bucket>/<key> -` が成功すれば十分です）

ここでは、次の Terraform 構成がすでに apply 済みだとします。

```hcl
# network/ (backend "s3" { bucket = "example-tfstate", key = "network/terraform.tfstate" })
output "vpc_id" {
  value = aws_vpc.main.id
}

output "private_subnet_ids" {
  value = aws_subnet.private[*].id
}

# database/ (backend "s3" { bucket = "example-tfstate", key = "database/terraform.tfstate" })
output "master_password" {
  value     = random_password.master.result
  sensitive = true
}
```

## 1. 参照する tfstate を宣言する

`tazuna.yaml` の `spec.tfstates[]` に、参照する tfstate を名前付きで並べます。
`bucket` / `key` / `region` は terraform の backend 設定と同じ値を書きます。

```yaml
apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  tfstates:
    - name: network
      backend:
        type: s3
        s3:
          bucket: example-tfstate
          key: network/terraform.tfstate
          region: ap-northeast-1
    - name: database
      backend:
        type: s3
        s3:
          bucket: example-tfstate
          key: database/terraform.tfstate
          region: ap-northeast-1
  manifests: []
```

認証情報は書きません。AWS CLI と同じように `AWS_PROFILE` や環境変数から解決されます。

## 2. helmfile に値を渡す

`helmfile.vars` で `from: tfstate` を使います。`address` には `output.<name>` のほか、
`aws_vpc.main.id` のような resource の attribute も指定できます。

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
              address: output.private_subnet_ids
```

helmfile 側では、ほかの vars と同じく `.StateValues` から参照します。list は list のまま
渡されます。

```yaml
# helmfile/aws-load-balancer-controller/helmfile.yaml.gotmpl
releases:
  - name: aws-load-balancer-controller
    namespace: kube-system
    chart: eks/aws-load-balancer-controller
    values:
      - vpcId: {{ .StateValues.vpcId | quote }}
        subnets:
{{ .StateValues.subnetIds | toYaml | indent 10 }}
```

## 3. sensitive な値を Secret にする

パスワードのような sensitive な output は、helmfile vars ではなく GenesisSecret で
Kubernetes Secret にします。`type: tfstate` の provider を宣言します。

```yaml
spec:
  providers:
    - name: tf-database
      type: tfstate
      tfstate:
        source: database
  manifests:
    - name: database-secret
      type: genesissecret
      path: ./genesissecrets/database.yaml
```

GenesisSecret の `items` のキーには、tfstate のアドレスを書きます。

```yaml
# genesissecrets/database.yaml
apiVersion: tazuna.pepabo.com/v1
kind: GenesisSecret
spec:
  provider: tf-database
  secrets:
    - items:
        output.master_password:
          mapTo: DB_PASSWORD
  outputs:
    - kubernetesSecret:
        namespace: app
        name: database
```

## 4. 確認して適用する

```bash
tazuna check                         # 設定の検査（backend にはアクセスしない）
tazuna build --log-level debug       # 値を解決して render 結果を確認する
tazuna apply
```

`--log-level debug` を付けると、どの tfstate をどこから何回読んだかがログに出ます
（値そのものは出ません）。

```text
level=DEBUG msg="tfstate: loading state" source=network backend=s3 bucket=example-tfstate key=network/terraform.tfstate region=ap-northeast-1
level=DEBUG msg="tfstate: loaded state" source=network backend=s3 ... serial=13 terraform_version=1.10.5 outputs=2 resources=12 elapsed=182ms
level=DEBUG msg="tfstate: resolved value" source=network address=output.vpc_id value_kind=string
```

アドレスを間違えたときは、候補の一覧を含むエラーになります。

```text
failed to resolve helmfile var vpcId from tfstate: tfstate source "network": failed to look up "output.vpcid":
  output "vpcid" not found in tfstate (available outputs: private_subnet_ids, vpc_id)
```

## 応用: 環境ごとに workspace を切り替える

Terraform の workspace で環境を分けている場合は、`workspace` に
[`{{ .Environment }}`](./environments.md) を書くと、`-e` に合わせて読む tfstate を切り替えられます。

```yaml
  tfstates:
    - name: network
      backend:
        type: s3
        s3:
          bucket: example-tfstate
          key: network/terraform.tfstate
          workspace: {{ .Environment }}   # -e prod なら env:/prod/network/terraform.tfstate
          region: ap-northeast-1
```

## 応用: 任意のフィールドに埋め込む

helmfile vars 以外のフィールド（kustomize の `defaultNamespace` や `tags` など）には、
template 関数 `tfstate` / `tfstateJSON` を使います。

```yaml
    - name: app
      type: kustomize
      path: ./kustomize/app
      kustomize:
        defaultNamespace: {{ tfstate "network" "output.app_namespace" }}
```

template 関数は `tazuna.yaml` を読み込んだ時点で評価されるため、`tazuna check` を含む
すべてのコマンドで tfstate を読みに行きます。オフラインで `check` したい場合は、
helmfile vars と GenesisSecret だけを使ってください。

## まとめ

- `spec.tfstates[]` で参照する tfstate に名前を付ける。
- helmfile には `from: tfstate`、Secret には `type: tfstate` の provider、
  それ以外のフィールドには template 関数を使う。
- tfstate は 1 回の実行につき 1 回だけ読み込まれ、値はログに出ない。
