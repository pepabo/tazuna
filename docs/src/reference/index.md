# リファレンス

このセクションは、Tazuna が受け付ける入力ファイルや CLI、内部データ構造の**仕様**を、
規約書として参照しやすい形でまとめます。

「なぜそうなっているか」は [概念](../concepts/index.md)、
「どう使うか」の手順は [ガイド](../guides/index.md) を参照してください。
リファレンスは事実の列挙に徹し、フィールド・型・デフォルト・例を中心に書きます。

## 一覧

現在掲載しているリファレンスは次のとおりです。
ここから順次、Manifest type 別の詳細や CLI、Test plugin、State の内部構造などを拡充していきます。

- **[`tazuna.yaml` スキーマ](./tazuna-yaml.md)** —
  Tazuna への唯一の入力ファイルである `tazuna.yaml` のトップレベル構造と、
  `spec.manifests[]` / `spec.context_matches` / `includes` などの共通フィールドの仕様。
- **[`tazuna.hint.yaml` スキーマ](./tazuna-hint-yaml.md)** —
  helmfile Manifest の `vars` に対する制約を宣言するヒントファイルのスキーマ。
  型・必須・条件付き必須・フォーマット検証ルールと、`oneof_required` などのトップレベルルール。
- **[GenesisSecret スキーマ](./genesis-secret.md)** —
  外部 Secret ストア（1Password）から Kubernetes Secret を生成するための YAML スキーマ。
  `type: genesissecret` の Manifest として `tazuna.yaml` から参照されます。
- **[Test plugin](./test-plugin.md)** —
  `manifests[].tests` および `spec.tests` に書く `TestPluginSpec` の共通フィールドと、
  組み込みプラグイン `WaitUntil` / `ExistNonExist` の仕様。
- **[State の内部構造](./state.md)** —
  State の保存先（`tazuna` namespace の ConfigMap）、State key の文字列形式、
  ContentHash の計算ルール、Diff type の分類仕様。
- **[Manifest type 別](./manifest-types/index.md)** —
  `kustomize` / `helmfile` / `oras` / `parallel` / `genesissecret` の 5 type について、
  `path` の意味・固有フィールド・apply / destroy / build 時の振る舞いを 1 ページずつ。
- **[CLI](./cli/index.md)** —
  `tazuna` バイナリのサブコマンド・グローバルフラグ・環境変数の仕様。
  各サブコマンドは 1 ページずつに分けて、フラグと振る舞いをまとめています。

## 読み方の約束

- フィールド名は YAML での表記（小文字キャメル or スネーク）で示します。
- **必須** と注記のないフィールドはすべて optional です。
- 「デフォルト」は値を省略したときに Tazuna が採用する値を示します。
  ゼロ値（空文字 / 空スライス / `false` / `0`）はとくに注記しない限りそのまま採用されます。
- 例示する YAML は最小構成で書きます。
  実運用で必要になる追加フィールドは各セクションで個別に説明します。
