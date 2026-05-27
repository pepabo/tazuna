# コントリビュート

このセクションは、Tazuna のコードベース・ドキュメント・リリースに変更を入れる人向けの案内です。
リポジトリのルートにある [`CONTRIBUTING.md`](https://github.com/pepabo/tazuna/blob/main/CONTRIBUTING.md)
が一次情報で、ここではそれを補う形で各トピックを 1 ページずつにまとめます。

## 一覧

- **[開発環境](./development.md)** —
  `mise` で toolchain を揃え、`make build` でローカルバイナリを作るまでの手順とリポジトリ構成。
- **[テスト](./testing.md)** —
  unit / integration / e2e の 3 レイヤと `make` ターゲットの対応、KinD クラスタの用意。
- **[ドキュメント](./documentation.md)** —
  `docs/` の構造、`mdbook` でのプレビュー、`po/en.po` を使った英語訳の更新フロー、GitHub Pages への公開。
- **[リリース](./releases.md)** —
  tag push を起点とした goreleaser によるリリース、バージョン埋め込み、SBOM / 署名 / 来歴。

## バグ報告 / 機能提案

GitHub の [Issue テンプレート](https://github.com/pepabo/tazuna/tree/main/.github/ISSUE_TEMPLATE)
を使ってください。テンプレート無しの自由記述 issue は受け付けます。

セキュリティ起因の問題は [`SECURITY.md`](https://github.com/pepabo/tazuna/blob/main/SECURITY.md)
の手順に従ってください。**公開 issue は作らないでください。**

## Pull Request の流れ

CONTRIBUTING.md の記述と同じ流れです。再掲しておきます。

1. `main` から作業ブランチを切る。
2. 変更はトピックごとに小さくまとめる。
3. push する前にローカルで `make test` と `make lint` を通す。
4. `main` 宛に PR を作成。CI が green になるまでレビュー対象にならない。

PR テンプレートはリポジトリの [`.github/PULL_REQUEST_TEMPLATE.md`](https://github.com/pepabo/tazuna/blob/main/.github/PULL_REQUEST_TEMPLATE.md)
を使います。
