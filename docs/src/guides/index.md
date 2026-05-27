# ガイド

このセクションでは、Tazuna を使って実際に手を動かすための手順をまとめます。
[概念](../concepts/index.md) が「なぜそうなっているか」を扱うのに対し、
ここでは「何を、どの順で、どのコマンドで行うか」をタスク単位で示します。

各ガイドは独立して読めるように書いていますが、まだ Tazuna に触れたことがない場合は、
順番に通すと無理なく進められます。コマンドの細かい挙動やフラグの一覧は
[リファレンス](../reference/index.md) を参照してください。

## 入門

新しく Tazuna を導入したいときに最初に通すグループです。

1. **[最初の tazuna.yaml を書く](./first-tazuna-yaml.md)** —
   1 つの Kubernetes クラスタに kustomize で書いた add-on を 1 つ入れるところまでを、
   `tazuna.yaml` の最初の 1 枚から `tazuna apply` までひと通り通します。

これ以降のテーマ（複数 manifest の順序付け、`--tags` による絞り込み、
State の確認、GenesisSecret、CI 連携など）は、ここで作った `tazuna.yaml` を
徐々に拡張していく形で順次追加していきます。それまでの間、各テーマの
**仕様** は以下のリファレンスから引けます。

- `--tags` の評価: [`tazuna.yaml` - tags](../reference/tazuna-yaml.md#tags)
- State の確認: [State の内部構造](../reference/state.md) / [`tazuna state list`](../reference/cli/state-list.md)
- GenesisSecret: [GenesisSecret スキーマ](../reference/genesis-secret.md)
- CI 連携: [CI パイプライン](../operations/ci-pipeline.md)
