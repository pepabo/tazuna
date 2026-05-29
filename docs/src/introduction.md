# ようこそ！

Tazuna ドキュメントへようこそ。

Tazuna は、マルチクラスタ Kubernetes 環境のブートストラップライフサイクルを管理するための CLI ツールです。
`tazuna.yaml` に「どのクラスタへ、どのマニフェストを、どの順で適用するか」を宣言的に記述し、
その内容を `apply` / `destroy` / `plan` / `status` / `state` といったコマンドで一貫して運用できます。

主なコマンド:

- **`tazuna apply`** — `tazuna.yaml` をクラスタに反映し、State を書き戻します。
  `--sync` で差分のみ反映、`--sync --prune` で不要リソースの削除、`--sync --atomic` で
  途中エラー時の State 巻き戻しに対応します。
- **`tazuna plan`** — apply したらどのフィールドが変わるかを unified diff で見せます。
- **`tazuna status`** — State に記録された managed リソースの readiness を一覧します。
- **`tazuna state diff` / `tazuna state drift`** — 宣言 drift と live drift を別々に検知します。
- **`tazuna destroy`** — Tazuna 管理リソースをクラスタから取り外します。

## このドキュメントについて

このドキュメントは、Tazuna を初めて触る人から、CI に組み込んで継続的に運用する人、
そして Tazuna 本体に手を入れる人までを対象に、次の流れで構成しています。

- **[はじめかた](./getting-started/index.md)** — Tazuna を手元で初めて動かせる状態にするまで。
- **[概念](./concepts/index.md)** — Tazuna が解こうとしている問題と、その設計思想・アーキテクチャ。「なぜそうなっているか」を扱います。
- **[ガイド](./guides/index.md)** — `tazuna.yaml` を書くなど、実際に手を動かすためのタスク単位の手順。「何を、どの順で、どのコマンドで行うか」を扱います。
- **[運用](./operations/index.md)** — `destroy` の運用、drift モニタリング、CI パイプラインなど、継続的に使う局面の指針。
- **[リファレンス](./reference/index.md)** — 入力ファイル・CLI・内部データ構造の仕様。フィールド・型・デフォルト・例を規約書として参照できます。
- **[コントリビュート](./contributing/index.md)** — 開発環境・テスト・ドキュメント・リリースなど、Tazuna に変更を入れる人向けの案内。

## どこから読むか

- まだ Tazuna に触れたことがなければ、**[はじめかた](./getting-started/index.md)** から順に進むのがおすすめです。
- 設計の背景や用語を先に押さえたい場合は **[概念](./concepts/index.md)** から。
- 特定のコマンドやフィールドの仕様だけを調べたい場合は **[リファレンス](./reference/index.md)** を辞書的に参照してください。
