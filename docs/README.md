# Tazuna Documentation

This directory hosts the Tazuna documentation site, built with [mdBook] and
internationalised via [mdbook-i18n-helpers] (gettext / PO files).

The published site lives at <https://pepabo.github.io/tazuna/>.

## Layout

```
docs/
├── book.toml          # mdBook configuration
├── src/               # Source documents (authored in Japanese)
│   ├── SUMMARY.md
│   └── introduction.md
├── po/                # Translations
│   └── en.po          # English
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

Japanese (the source language):

```sh
cd docs
mdbook serve --open
```

English:

```sh
cd docs
MDBOOK_BOOK__LANGUAGE=en mdbook serve --open
```

## Build locally

```sh
cd docs
mdbook build -d book/ja
MDBOOK_BOOK__LANGUAGE=en mdbook build -d book/en
cp static/index.html book/index.html
```

Open `book/index.html` in a browser to verify the language switcher.

## Update translations

After editing files under `src/`, refresh the PO template and merge:

```sh
cd docs
MDBOOK_OUTPUT__XGETTEXT__POT_FILE=messages.pot \
  mdbook build -d po --no-create-missing
msgmerge --update po/en.po po/messages.pot
```

Then open `po/en.po` and translate the new `msgid` entries.

## Deployment

`.github/workflows/docs.yaml` handles deployment:

- On `push` to `main`, the site is built and deployed to GitHub Pages via the
  official `actions/upload-pages-artifact` + `actions/deploy-pages` flow.
- On pull requests, only the build runs. The resulting site is available as a
  `github-pages` artifact on the workflow run — download and extract it locally
  to preview changes before merging. Live URL previews are intentionally not
  provided, since GitHub Pages only supports a single deployment per site.

## Third-party assets

The site loads the [M PLUS U](https://fonts.google.com/specimen/M+PLUS+U) font
from Google Fonts at runtime. See [THIRDPARTY.md](./THIRDPARTY.md) for license
and attribution details. Add an entry there whenever you introduce a new font,
icon set, or other third-party asset.

## One-time Pages setup

After merging the initial workflow, configure the repository once:

1. Go to **Settings → Pages**.
2. Set **Source** to *GitHub Actions*.
3. Push to `main` (or re-run the workflow on `main`). The `deploy` job creates
   the `github-pages` environment on its first successful run.
4. Wait for the workflow to finish, then open <https://pepabo.github.io/tazuna/>.

[mdBook]: https://rust-lang.github.io/mdBook/
[mdbook-i18n-helpers]: https://github.com/google/mdbook-i18n-helpers
