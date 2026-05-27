# `tazuna check`

`tazuna.yaml` の妥当性を、クラスタには触れずに検証します。
CI で最初に通す対象に向きます。

```text
tazuna check [-f tazuna.yaml] [--fix]
```

## 振る舞い

1. `tazuna.yaml` をロードする。
2. ファイルおよび展開後の `manifests[]` 全体に対してバリデーションを実行する。
3. 問題が無ければ `ok` を標準出力に書き、終了コード 0 で抜ける。
4. `--fix` を指定した場合は、`name` 未設定の Manifest に自動採番した上で
   `tazuna.yaml` を書き戻し、`fixed: <path>` を標準出力に書く。

検証項目の一覧は [`tazuna.yaml` スキーマ - バリデーションのまとめ](../tazuna-yaml.md#バリデーションのまとめ) を参照してください。
クラスタへのアクセスは行いません。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) に加えて次を受け付けます。

| フラグ    | エイリアス | 型   | デフォルト | 説明 |
|-----------|------------|------|------------|------|
| `--fix`   | -          | bool | `false`    | `name` 未設定の Manifest を自動採番し、`tazuna.yaml` を書き戻します。 |

`--fix` はファイルを上書きします。バージョン管理下で実行することを推奨します。

## 例

```bash
tazuna check
tazuna check -f path/to/tazuna.yaml
tazuna check --fix
```

## 関連

- 検証ルールの詳細: [`tazuna.yaml` スキーマ](../tazuna-yaml.md)
