# Drift モニタリング

このページは、`tazuna state diff` を **定期的に回して drift を可視化する運用** の作り方をまとめます。
コマンドの仕様は [`tazuna state diff`](../reference/cli/state-diff.md) を、
State の中身の仕様は [State の内部構造](../reference/state.md) を参照してください。

## 何を drift と呼ぶか

ここでの drift は、`tazuna.yaml` から生成されるべきリソース集合（**Build 結果**）と、
クラスタ内 State に記録されているリソース集合の差です。
これは `tazuna state diff` の出力そのものに当たります。

| Diff type     | 検出されるケース | drift の典型例 |
|---------------|------------------|----------------|
| `added`       | Build 結果に存在し、State に無い | `tazuna.yaml` を更新して Manifest を増やしたが、まだ反映していない |
| `modified`    | 両方に存在するが内容が違う       | Helm values 変更、kustomize overlay 変更、image tag 更新の未反映 |
| `removed`     | State にあって Build 結果に無い  | Manifest を `tazuna.yaml` から外したが、リソースはクラスタに残っている |
| `always-sync` | 常に同期扱い                     | GenesisSecret 由来 Secret。drift ではなく「毎回チェックする箇所」 |

`tazuna state diff` は **クラスタの実体までは見ていません**。
クラスタに対して手で `kubectl apply` した結果（State には無いリソース）は
ここでは検出されません。Tazuna の管理対象外として無視されます。

## 出力フォーマット

`tazuna state diff` は Manifest 単位で次のような出力を出します。

```text
Manifest: ingress-nginx
  STATUS         RESOURCE                                                     HASH
  modified       ingress-nginx/apps/v1/Deployment/ingress-nginx/controller    abc123... -> def456...

Manifest: aws-credentials
  STATUS         RESOURCE                                                     HASH
  always-sync    aws-credentials//v1/Secret/default/aws-credentials           xyz789...
```

差分が無い場合は次の 1 行だけが出ます。

```text
No changes detected.
```

「drift なし」の判定は **この 1 行で判断するのが現状もっとも素朴** です
（出力に `No changes detected.` を含むかどうかでフィルタする）。
`tazuna state diff` 自体は差分の有無で終了コードは変えません。
差分があってもエラーにはならない、という点に注意してください。

## 監視のかたち

実運用での「drift モニタリング」は次のいずれか（または組み合わせ）になります。

### a. CI ジョブを定期実行する

GitHub Actions の `schedule` で 1 日に数回 `tazuna state diff` を回し、出力を保存します。

- **メリット**: 既存の CI 認証を再利用できる。差分が出たら Slack 等に投げやすい。
- **デメリット**: クラスタ接続情報を CI に持ち込む必要がある。短い周期には向かない。

ポイント:

- ジョブには **クラスタへの read 権限だけ** あれば足ります（`tazuna state diff` は
  クラスタを変更しません）。
- 出力を `tazuna state diff -f path/to/tazuna.yaml > diff.txt` のようにファイルに落とし、
  `No changes detected.` を含まない場合だけ通知を投げると、無風時のノイズが消えます。

### b. クラスタ内 Job として走らせる

`tazuna` バイナリを含むコンテナイメージを用意し、CronJob として定期実行する方法です。

- **メリット**: 認証は ServiceAccount に閉じる。短い周期にしやすい。
- **デメリット**: イメージのビルド・配布が必要。CI と同じ `tazuna.yaml` リポジトリへの
  アクセスを Job 側にも持たせる必要がある。

`type: oras` を使って `tazuna.yaml` 一式を OCI artifact として配布しておくと、Job 側で
リポジトリの clone を持たずに済みます（[`tazuna apply --offline`](../reference/cli/index.md#環境変数)
と組み合わせると registry も不要になります）。

## 通知の組み立て

通知側で読みたい情報は次の 3 つです。

- どの **Manifest** に差分があるか
- どの **Diff type** か（特に `removed` は注意）
- どの **リソース** か（State key の形式で）

State key の形式は `manifest/group/version/kind/namespace/name`（cluster-scoped は
`namespace` 抜き）で固定なので、grep ベースの後段処理に十分なります。
詳細は [State の内部構造 - State key](../reference/state.md#state-key) を参照してください。

通知の最小プロトタイプ:

```bash
if ! tazuna state diff -f tazuna.yaml | tee diff.txt | grep -q "No changes detected."; then
  curl -X POST "$SLACK_WEBHOOK_URL" --data "$(jq -Rs '{text: .}' < diff.txt)"
fi
```

`jq -Rs '{text: .}'` は、`diff.txt` の中身を **生文字列のまま** Slack の Incoming Webhook が
期待する `{"text": "..."}` 形式 JSON に包み直すための定型です（`-R` で raw 入力、`-s` で
全行を 1 つの文字列にスラープ）。

## 検知後の対応

drift が出たときの選択肢は次のいずれかです。

- **意図した変更だった**: `tazuna apply`（または `tazuna state sync`）で State をクラスタに追従させる。
- **意図しない変更だった**:
  - **`modified`**: 誰がいつ変えたかを git log / クラスタの監査ログ等で追い、
    変更を巻き戻すか `tazuna.yaml` 側に取り込むかを判断する。
  - **`added`**: `tazuna.yaml` 側に Manifest を増やしたが反映していない、というケースが多い。
    意図に合わせて apply するか、`tazuna.yaml` 側を元に戻す。
  - **`removed`**: Tazuna から Manifest を外したが、リソースはクラスタに残っている。
    [`tazuna destroy`](../reference/cli/destroy.md) の `--tags` 絞り込みや、
    `tazuna state sync` + `TAZUNA_STATE_SYNC_DELETE=true` で片付ける。
- **GenesisSecret の `always-sync`**: drift ではないので通知から除外して構いません。

## 関連

- コマンド仕様: [`tazuna state diff`](../reference/cli/state-diff.md) /
  [`tazuna state sync`](../reference/cli/state-sync.md)
- State の内部構造: [State の内部構造](../reference/state.md)
- 用語: [Diff type](../concepts/glossary.md#diff-type) /
  [always-sync](../concepts/glossary.md#always-sync)
