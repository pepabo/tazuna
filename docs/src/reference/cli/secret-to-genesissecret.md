# `tazuna secret-to-genesissecret`

クラスタ上の既存 Secret を 1Password に書き出し、
それを参照する GenesisSecret YAML を生成します。
**移行 / 棚卸し用の片道のコマンド** であり、定常運用で繰り返し叩くものではありません。

```text
tazuna secret-to-genesissecret \
  --op-host <host> \
  [--namespace <ns>] \
  [--label-selector <sel>] [--name-regex <re>] \
  [--vault <vault>] [--note <note>] \
  [--dump-dir <dir>] [--dry-run]
```

## 振る舞い

1. `--namespace`（デフォルト `default`）の Secret を `--label-selector` /
   `--name-regex` で絞り込む。
2. 各 Secret のデータを 1Password の `--vault` に Item として書き出す。
3. その Item を参照する GenesisSecret YAML を `--dump-dir`（デフォルト `.`）に出力する。
4. `--dry-run` のときは、1Password への書き込みも YAML の生成も行わずに、
   対象 Secret の選定結果だけを出力する。

`tazuna.yaml` は読まないので `-f` / `--file-path` は無視されます。
グローバルフラグのうち実際に効くのは `-l` / `--log-level` だけです。
クラスタへの read と 1Password への write の両方が走るため、
**1Password CLI (`op`) が認証済みであること** が前提です。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) に加えて次を受け付けます。

| フラグ              | 型     | デフォルト  | 必須 | 説明 |
|---------------------|--------|-------------|------|------|
| `--op-host`         | string | -           | ◯    | 1Password サービスアカウント URL のホスト部分（例: `example.1password.com`）。 |
| `--namespace`       | string | `default`   | -    | 対象 Secret が存在する Kubernetes namespace。シェル補完で実クラスタの namespace を列挙します。 |
| `--label-selector`  | string | `""`        | -    | 対象 Secret を絞る label selector。例: `app=foo,tier=db`。 |
| `--name-regex`      | string | `""`        | -    | 対象 Secret の name に対する正規表現。 |
| `--vault`           | string | `""`        | -    | 1Password の vault 名。シェル補完で実 vault を列挙します。 |
| `--note`            | string | `""`        | -    | 生成される 1Password Item に付ける note。 |
| `--dump-dir`        | string | `.`         | -    | 生成された GenesisSecret YAML の出力先ディレクトリ。 |
| `--dry-run`         | bool   | `false`     | -    | 書き込みを行わず、選定結果だけを出力します。 |

## 例

```bash
tazuna secret-to-genesissecret \
  --op-host example.1password.com \
  --namespace production \
  --label-selector tazuna.pepabo.com/migrate=true \
  --vault example-vault \
  --dump-dir ./genesissecrets

tazuna secret-to-genesissecret \
  --op-host example.1password.com \
  --name-regex '^db-.*' \
  --dry-run
```

## 関連

- 生成された YAML は `type: genesissecret` の Manifest として `tazuna.yaml` から参照します。
- 用語: [GenesisSecret](../../concepts/glossary.md#genesissecret) /
  [Provider (SecretProvider)](../../concepts/glossary.md#provider-secretprovider)
