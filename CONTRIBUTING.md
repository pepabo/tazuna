# Contributing to tazuna / 貢献ガイド

Thanks for taking the time to contribute!
コントリビュートに興味を持っていただきありがとうございます。

## Development setup / 開発環境

```bash
make format             # gofmt
make test               # unit tests
make test-integration   # integration tests (build tag: integration)
make test-e2e           # end-to-end tests (requires a KinD cluster)
make lint               # golangci-lint
```

E2E tests need a KinD cluster. Spin one up with:
E2E テストには KinD クラスタが必要です:

```bash
make devenv-create
make devenv-destroy
```

## Pull Request flow / PR の流れ

1. Fork the repo and create a feature branch from `main`.
   `main` から作業ブランチを切ってください。
2. Make your changes. Keep commits focused.
   変更はトピックごとに小さくまとめてください。
3. Run `make test` and `make lint` locally before pushing.
   push する前にローカルで `make test` と `make lint` を通してください。
4. Open a PR against `main`. CI must be green before review.
   `main` 宛に PR を作成してください。CI が green であることがレビュー前提です。

## Reporting bugs and proposing features / バグ報告・機能提案

Use the Issue templates under [.github/ISSUE_TEMPLATE/](.github/ISSUE_TEMPLATE/).
Issue テンプレートを利用してください。

For security issues, follow [SECURITY.md](./SECURITY.md) instead — **do not open a public issue**.
セキュリティ問題は [SECURITY.md](./SECURITY.md) を参照してください。**公開 issue にはしないでください**。
