# ドキュメント

このページは、Tazuna のドキュメントサイト（いま読んでいるサイト）に変更を入れる人向けの案内です。
サイトは `docs/` 配下で完結しており、[mdBook](https://rust-lang.github.io/mdBook/) +
[mdbook-i18n-helpers](https://github.com/google/mdbook-i18n-helpers)（gettext / PO ファイル）
で構築しています。

ソースは **日本語** で書き、英語版は `po/en.po` の翻訳で派生させる構成です。

## ディレクトリ構成

```text
docs/
├── book.toml            # mdBook の設定
├── src/                 # ドキュメント本体（日本語ソース）
│   ├── SUMMARY.md
│   ├── introduction.md
│   ├── concepts/
│   ├── getting-started/
│   ├── guides/
│   ├── operations/
│   ├── reference/
│   └── contributing/
├── po/
│   └── en.po            # 英語訳
├── theme/
│   └── fonts.css        # Google Fonts (M PLUS U) の上書き
├── static/
│   └── index.html       # /en/ と /ja/ をリンクする landing
└── THIRDPARTY.md        # フォント等のサードパーティ資産
```

`docs/src/SUMMARY.md` がページの目次そのものです。新しいページを追加するときは
ここにも 1 行追記する必要があります（追記漏れがあると mdBook はビルドできても、
そのページはサイトに出ません）。

## 必要なツール

```bash
cargo install mdbook --locked
cargo install mdbook-i18n-helpers --locked
```

`msgmerge`（gettext 同梱）は PO の更新で使います。macOS なら `brew install gettext`。

## ローカルでプレビュー

日本語版（ソース言語）:

```bash
cd docs
mdbook serve --open
```

英語版:

```bash
cd docs
MDBOOK_BOOK__LANGUAGE=en mdbook serve --open
```

ファイル保存のたびに再ビルドされてブラウザがリロードされます。

## ローカルでビルド

```bash
cd docs
mdbook build -d book/ja
MDBOOK_BOOK__LANGUAGE=en mdbook build -d book/en
cp static/index.html book/index.html
```

`book/index.html` をブラウザで開けば、リンクで `/ja/` と `/en/` を切り替えられます。

## 英語訳の更新

`docs/src/` 配下の文章を編集したら、PO テンプレートを再生成して `en.po` に merge します。

```bash
cd docs
MDBOOK_OUTPUT__XGETTEXT__POT_FILE=messages.pot \
  mdbook build -d po --no-create-missing
msgmerge --update po/en.po po/messages.pot
```

このあと `po/en.po` を開いて、追加された `msgid` に対応する `msgstr` を埋めます。
ソース側（日本語）と英語訳の対応が崩れているとサイト上で空白や欠落が見えるので、
ソース変更と同じ PR で `en.po` も更新するのが基本フローです。

## デプロイ

`.github/workflows/docs.yaml` が公開を担当します。

- `main` への push: サイトをビルドし、GitHub Pages へ公開する
  （`actions/upload-pages-artifact` + `actions/deploy-pages`）
- PR: ビルドのみ。生成物は `github-pages` という名前で workflow run の artifact として
  ダウンロードできます。ローカルで展開して目で見るのが推奨レビュー手順です。

GitHub Pages は 1 サイトにつき 1 デプロイメントしか持てないため、PR ごとの live
プレビュー URL は意図的に提供していません。

## サードパーティ資産

サイトは Google Fonts の [M PLUS U](https://fonts.google.com/specimen/M+PLUS+U) を
ランタイムでロードしています。フォント / アイコン / 画像など新しい外部資産を入れる場合は、
[`docs/THIRDPARTY.md`](https://github.com/pepabo/tazuna/blob/main/docs/THIRDPARTY.md)
にライセンスと帰属を追記します。

## ドキュメント側の規約

ここまでに書いた既存ページが従っている規約を、参考として残しておきます。

- **トーンの使い分け**:
  概念は散文中心（[concepts/](../concepts/index.md)）、ガイドは「目的→前提→手順」、
  リファレンスはフィールド表とコード断片中心。
- **アンカーリンク**:
  glossary や他リファレンスへの導線は `#anchor` まで含めて貼る。
  例: `[Manifest type](../concepts/glossary.md#manifest-type)`。
- **コードと識別子**:
  `tazuna.yaml` のフィールド名、CLI フラグ、Go の型名はバッククォートで囲む。
- **言語ポリシー**:
  ソースは日本語で書く。コード中のコメントは日本語で書いてよいが、
  ログ・エラーメッセージ・CLI 出力は英語に統一する（プロジェクト全体の方針）。
