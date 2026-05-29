# CI パイプライン

このページは、`tazuna.yaml` を持つリポジトリで **CI / CD パイプラインに Tazuna を組み込む**
ときの典型構成をまとめます。Tazuna は手元で `apply` するためにも、CI から
`apply` するためにも使えます。ここでは後者の組み立て方を中心に扱います。

## 典型構成

役割を分けると次の 3 ステージになります。

| ステージ | 目的 | 走らせるコマンド | クラスタアクセス |
|----------|------|------------------|------------------|
| **検証** | `tazuna.yaml` が壊れていないことの保証 | `tazuna check`、`tazuna build`、`tazuna plan` | `plan` のみあり（read-only） |
| **反映** | `main` の内容をクラスタに反映する | `tazuna apply`（必要に応じて `--sync` / `--prune`） | あり |
| **取り外し** | Tazuna 管理リソースを削除する | `tazuna destroy` | あり |

「検証」は全 PR で走らせて構いません。「反映」は通常 `main` への push をトリガにします。
「取り外し」は **CI に常設しない** ことを推奨します（[`tazuna destroy` の運用](./destroy-runbook.md) 参照）。

## 検証ステージ

`tazuna.yaml` を CI に乗せる場合、最低限の通過条件として `tazuna check` を入れます。
クラスタに触れずに走るので、PR でのコストはほぼゼロです。

```yaml
# GitHub Actions の例（要点だけ）
- name: tazuna check
  run: tazuna check -f tazuna.yaml

- name: tazuna build (preview)
  run: tazuna build -f tazuna.yaml > rendered.yaml
```

PR で `build` 結果をアーティファクトに残しておくと、レビュー時に「最終的に何が apply されるか」を
レンダリング結果ベースで確認できます。`type: oras` を使っているなら `--offline` を
付けて先取り cache を使う構成も検討できます（[`tazuna build`](../reference/cli/build.md)）。

クラスタへの read 権限を CI に渡せる場合は、`tazuna plan` を PR で回しておくと
「実際に何のフィールドが変わるか」を unified diff で PR コメントに貼れます。

```yaml
- name: tazuna plan
  if: github.event_name == 'pull_request'
  run: tazuna plan -f tazuna.yaml > plan.txt
- name: post plan to PR
  if: github.event_name == 'pull_request'
  run: |
    gh pr comment "$PR_NUMBER" --body-file <(printf '```\n%s\n```\n' "$(cat plan.txt)")
```

`tazuna plan` は読み取り専用なので、PR の権限境界に乗せやすいのが利点です。

## 反映ステージ

`main` への push をトリガに `tazuna apply` を走らせる構成が基本です。

```yaml
on:
  push:
    branches: ["main"]

jobs:
  apply:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write   # OIDC でクラスタに繋ぐ場合
    steps:
      - uses: actions/checkout@v6
      - name: install tazuna
        run: |
          curl -L https://github.com/pepabo/tazuna/releases/download/v0.1.0/tazuna_Linux_x86_64.tar.gz \
            | tar xz -C /usr/local/bin tazuna
      - name: configure kubeconfig
        run: |
          # クラスタ側の都合に応じて、aws-iam-authenticator / gke-gcloud-auth-plugin
          # / kubeconfig secret 等で current-context を設定する
          aws eks update-kubeconfig --name prod-tokyo --region ap-northeast-1
      - name: tazuna apply
        run: tazuna apply -f tazuna.yaml
```

ポイント:

- `tazuna apply` の current-context は kubeconfig の current-context そのものです。
  CI で current-context を切り替えるステップを必ず明示します。
- `tazuna.yaml` に [`spec.context_matches`](../reference/tazuna-yaml.md#context_matches) を
  入れておけば、誤って違うクラスタを向いた kubeconfig で apply しようとしても
  即終了します。CI でも有効な保険になります。
- Tazuna は失敗時に非ゼロ終了するので、CI 側で特別なエラーハンドリングは不要です
  （[CLI - 終了コード](../reference/cli/index.md#終了コード)）。

### `apply` の動作モード

`tazuna apply` には 3 つの動作モードがあります（詳細は
[`tazuna apply`](../reference/cli/apply.md#state-連携---sync----prune----atomic) を参照）。

| モード                       | いつ何が走るか                                                          | drift 検知 |
|------------------------------|-------------------------------------------------------------------------|------------|
| `tazuna apply`               | `tazuna.yaml` の全 Manifest を Manager に通し、state を保存             | しない（毎回上書き） |
| `tazuna apply --sync`        | Build 結果と State を比較し、差分（`added` / `modified` / `always-sync`）だけ反映 | する |
| `tazuna apply --sync --prune`| 上記に加え、State にあって Build 結果に無いリソースを削除               | する |

おおまかな指針:

- ブートストラップ初期 / Manifest 数が少ない: `tazuna apply` が単純で予測しやすい。
- Manifest 数が増えて 1 回の `apply` が重くなった: `--sync` で差分のみに絞る。
- `removed` の自動削除が欲しい: `--sync --prune`。誤削除のリスクと相談。

### `--atomic` を使うか

`tazuna apply --sync --atomic` を付けると、いずれかのリソースでエラーが出たときに
State を更新せず終了します。**反映自体は途中まで進む** ため、CI で「全部入ったか
何も入らなかったか」の二値にはなりませんが、State 上の整合性は守れます。
詳細は [`tazuna apply`](../reference/cli/apply.md#state-連携---sync----prune----atomic) を参照。

## 取り外しステージ

CI から `tazuna destroy` を回す運用は推奨しません。

- 環境変数 `TAZUNA_DESTROY_EXECUTABLE=true` を CI に常設すると、誤発火時のガードが
  プロンプトしか残らなくなる（しかも CI ではプロンプトに答えられないので、`--force`
  と組み合わさると無条件で消えます）。
- どうしても CI から消す必要がある場合は、**手動トリガの専用 workflow** を作り、
  その job に限って環境変数を渡す構成にします。

詳細は [`tazuna destroy` の運用](./destroy-runbook.md) を参照してください。

## Tag を使った段階的反映

`tazuna apply --tags` で、CI で反映する Manifest を絞り込めます。
たとえば「インフラレイヤと application レイヤを分けて回す」「実験 add-on を独立した job
で回す」といった段階的反映に向きます。

```bash
tazuna apply --tags infra        # 先に基盤を入れる
tazuna apply --tags application  # 次にアプリ側
```

タグの設計は `tazuna.yaml` 側の [`manifests[].tags`](../reference/tazuna-yaml.md#tags) で
行います。

## 監視・通知との接続

CI とは別系で、`tazuna state diff` の定期実行を回しておくのが推奨構成です。
詳細は [Drift モニタリング](./drift-monitoring.md) を参照してください。
CI で `apply` の成否を見るだけだと、「`tazuna.yaml` の更新が無いのに drift が起きている」
ケースを取り逃がします。

## 関連

- 仕様: [`tazuna apply`](../reference/cli/apply.md) /
  [`tazuna build`](../reference/cli/build.md) /
  [`tazuna check`](../reference/cli/check.md) /
  [`tazuna plan`](../reference/cli/plan.md)
- 事故防止: [`tazuna destroy` の運用](./destroy-runbook.md)
- drift 検知: [Drift モニタリング](./drift-monitoring.md)
- tracing: [オブザーバビリティ](./observability.md)
