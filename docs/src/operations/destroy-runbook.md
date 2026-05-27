# `tazuna destroy` の運用

このページは、本番に近いクラスタで `tazuna destroy` を打つときの **手順と備えるべきガード** をまとめます。
コマンドの仕様そのものは [`tazuna destroy` リファレンス](../reference/cli/destroy.md) を参照してください。

## なぜ runbook 化するか

`destroy` は Tazuna 管理下のリソースをクラスタから取り除く操作で、影響範囲が
**クラスタ全体に及び得る** 唯一の系統的な書き込みコマンドです。手で `kubectl delete`
していくのと違い、Tazuna が「自分の管理対象」と見なすものはまとめて消えます。

二段ガード（プロンプト + 環境変数）は仕組みとしての防御線ですが、運用側でも
**「何が消えるかを事前に確認する」「context を取り違えない」** を組み立てておく必要があります。

## 前提として整える

destroy をクラスタに対して使う可能性があるなら、その `tazuna.yaml` には
**`spec.context_matches` を入れておく** ことを強く推奨します。

```yaml
spec:
  context_matches:
    - ^prod-tokyo$
  context_match_mode: or
  manifests:
    # ...
```

これがあると、`destroy` 時に current-context が `prod-tokyo` でなければ
クラスタには一切触れずに失敗します。手元の context 設定ミスに対する、もっとも安価な
保険です。詳細は [`tazuna.yaml` - context_matches](../reference/tazuna-yaml.md#context_matches) を参照。

## 標準フロー

以下の 1〜3 は **`tazuna destroy` の内部処理ではなく、運用上の事前確認手順** です。
Tazuna 自身は `state list` を呼びませんが、destroy 前に手で打つことを強く推奨します。

```bash
# 1. current-context を確認（destroy 直前は必ず）
kubectl config current-context
kubectl get nodes

# 2. 何が消えるかを把握する（推奨）
tazuna state list

# 3. 必要なら範囲を絞る（実行時とまったく同じ --tags を付ける）
tazuna state list --tags experimental   # 確認用

# 4. 本実行。プロンプトと環境変数の両方を満たして初めて削除が走る
TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy --tags experimental
```

`tazuna destroy` 自体は次の順で動きます。

1. `tazuna.yaml` をロード・バリデーションする。
2. `context_matches` を評価する。不一致なら即終了。
3. `--force` が無ければ Y/N のプロンプトを出す。
4. `TAZUNA_DESTROY_EXECUTABLE=true` でなければ、ログだけ出して終了。
5. すべて満たしていれば、Manager の Destroy を順に呼び出す。

「プロンプトに Yes」と「環境変数 `TAZUNA_DESTROY_EXECUTABLE=true`」の **両方** が
揃わない限り、リソースは消えません。

## 範囲を絞る運用

クラスタ全体ではなく、特定のグループだけを取り外したい場合は `--tags` で絞り込みます。

```bash
TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy --tags experimental
```

`--tags` は OR 評価です（[`tazuna.yaml` - tags](../reference/tazuna-yaml.md#tags)）。
廃止予定のものに `lifecycle:deprecated` のような専用タグを付けておき、その単位で削除する、
というパターンが扱いやすくなります。

`tazuna destroy --tags <空>` は全 Manifest が対象です。**絞らない destroy** を行うときは
事前に `tazuna state list` で全量を見ておき、想定外のリソースが入っていないことを確認してください。

## 事故が起きやすいシナリオ

実際にやりがちなパターンと、対応策の対応表です。

| シナリオ | どこで止まる / なぜ事故になるか | 推奨対応 |
|----------|--------------------------------|----------|
| current-context を staging のつもりが production になっていた | `context_matches` を書いていれば 2 で停止する。書いていないと進む。 | 本番系の `tazuna.yaml` には必ず `context_matches` を入れる。プロンプトで止まれるよう `--force` は付けない。 |
| CI に destroy を組み込み、誤って `main` で発火 | 環境変数を付けていないなら 4 で止まる。`TAZUNA_DESTROY_EXECUTABLE=true` を CI 全体に常設していると進んでしまう。 | CI には destroy を組み込まない。どうしても必要なら、専用の手動 workflow を作り、その job だけで一時的に環境変数を渡す。 |
| `--tags` を付け忘れて全消ししてしまう | プロンプトと環境変数の両方を満たすと進む。 | 「絞りなしの destroy」を本番に近いクラスタで打たない運用にする。打つときは事前 `state list` を必ず通す。 |
| 手作業で `kubectl apply` したものは destroy で消えない | State に無いリソースは Tazuna 管理対象外として扱われ、`destroy` の対象にならない。 | 状態が乖離していると判断したら、`tazuna state diff` で乖離を見える化した上で対応を決める。 |

## destroy の代わりに使える緩い手段

リソースを「いま完全に消す」必要がない場合、次の選択肢があることを覚えておきます。

- **`tazuna state sync` + `TAZUNA_STATE_SYNC_DELETE=true`** —
  `tazuna.yaml` から Manifest を消したうえで `state sync` を打つと、`removed` 分類で
  削除されます（[`tazuna state sync`](../reference/cli/state-sync.md)）。
  `tazuna.yaml` のソースの真実を変えずに reset したいときには使えません。
- **`--tags` で絞った destroy** — 上記参照。

## 関連

- コマンド仕様: [`tazuna destroy`](../reference/cli/destroy.md)
- 評価される `context_matches`: [`tazuna.yaml`](../reference/tazuna-yaml.md#context_matches)
- 削除前の確認: [`tazuna state list`](../reference/cli/state-list.md)
