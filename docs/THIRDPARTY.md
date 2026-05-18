# Third-party assets

This directory builds a documentation site that includes the following
third-party assets. Each is loaded from a CDN at runtime; no font files are
bundled in this repository. We list them here for attribution and to keep the
license terms easy to audit.

## Fonts

### M PLUS U

- Source: <https://fonts.google.com/specimen/M+PLUS+U>
- Upstream: <https://github.com/coz-m/MPLUS_FONTS>
- Copyright: Copyright 2021 The M+ FONTS Project Authors
- License: SIL Open Font License, Version 1.1 (OFL-1.1)
- License text: <https://openfontlicense.org/open-font-license-official-text/>

The font is delivered to users by Google Fonts (`fonts.googleapis.com` /
`fonts.gstatic.com`). The font files themselves are not redistributed from this
repository. The "Reserved Font Name" clause of OFL-1.1 applies to the name
"M PLUS U"; we use the font under its original name without modification.

## Adding a new third-party asset

When you introduce a new font, icon set, image, or similar asset, add an entry
here with: source URL, upstream project, copyright line, license identifier, and
a link to the license text. If the asset is bundled (not loaded from a CDN),
also commit the license file next to the asset.
