# Tazuna Documentation

This directory hosts the Tazuna documentation site, built with [mdBook] and
internationalised via [mdbook-i18n-helpers] (gettext / PO files).

The published site lives at <https://pepabo.github.io/tazuna/>.

## Layout

```
docs/
├── book.toml          # mdBook configuration
├── src/               # Source documents (authored in English)
│   ├── SUMMARY.md
│   └── introduction.md
├── po/                # Translations
│   └── ja.po          # Japanese
├── theme/
│   └── fonts.css      # Font override (M PLUS U via Google Fonts)
├── static/
│   └── index.html     # Landing page that links to /en/ and /ja/
└── THIRDPARTY.md      # Attribution for third-party fonts and assets
```

## Prerequisites

```sh
cargo install mdbook --locked
cargo install mdbook-i18n-helpers --locked
```

`msgmerge` (from gettext) is recommended for updating PO files. On macOS:
`brew install gettext`.

## Preview locally

English (the source language):

```sh
cd docs
mdbook serve --open
```

Japanese:

```sh
cd docs
MDBOOK_BOOK__LANGUAGE=ja mdbook serve --open
```

## Build locally

```sh
cd docs
mdbook build -d book/en
MDBOOK_BOOK__LANGUAGE=ja mdbook build -d book/ja
cp static/index.html book/index.html
```

Open `book/index.html` in a browser to verify the language switcher.

## Update translations

After editing files under `src/`, refresh the PO template and merge:

```sh
cd docs
MDBOOK_OUTPUT__XGETTEXT__POT_FILE=messages.pot \
  mdbook build -d po --no-create-missing
msgmerge --update po/ja.po po/messages.pot
```

Then open `po/ja.po` and translate the new `msgid` entries.

## Deployment

`.github/workflows/docs.yaml` handles deployment:

- On `push` to `main`, the built site is published to the `gh-pages` branch.
- On pull requests, a preview is published to
  `https://pepabo.github.io/tazuna/pr-preview/pr-<N>/` and posted as a comment
  on the PR. The preview is removed automatically when the PR is closed.

## Third-party assets

The site loads the [M PLUS U](https://fonts.google.com/specimen/M+PLUS+U) font
from Google Fonts at runtime. See [THIRDPARTY.md](./THIRDPARTY.md) for license
and attribution details. Add an entry there whenever you introduce a new font,
icon set, or other third-party asset.

## One-time Pages setup

After merging the initial workflow, configure the repository once:

1. Push to `main` so the workflow creates the `gh-pages` branch.
2. Go to **Settings → Pages**.
3. Set **Source** to *Deploy from a branch* and select `gh-pages` / `(root)`.
4. Wait a minute, then open <https://pepabo.github.io/tazuna/>.

[mdBook]: https://rust-lang.github.io/mdBook/
[mdbook-i18n-helpers]: https://github.com/google/mdbook-i18n-helpers
