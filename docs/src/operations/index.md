# 運用

このセクションは、Tazuna を **継続的に使う** 局面の指針をまとめます。
タスク単位の手順（「新しい add-on を入れる」「tazuna.yaml を書く」など）は
[ガイド](../guides/index.md) を、コマンドやスキーマの仕様は
[リファレンス](../reference/index.md) を参照してください。

このセクションでは、**事故を起こさない運用** と **drift を発見できる運用** を中心に扱います。

## 一覧

- **[`tazuna destroy` の運用](./destroy-runbook.md)** —
  本番クラスタで destroy を打つときに踏むべき手順、`TAZUNA_DESTROY_EXECUTABLE`
  と `context_matches` の二段ガード、事故が起きやすいシナリオ。
- **[Drift モニタリング](./drift-monitoring.md)** —
  `tazuna state diff` を定期実行して drift を可視化する運用、出力フォーマットと
  通知の組み立て方。
- **[CI パイプライン](./ci-pipeline.md)** —
  PR で `check` / `build`、`main` マージで `apply` を回す典型構成、
  `destroy` の置き場、状態同期の選択。
