# `tazuna init`

includes ベースの最小構成な `tazuna.yaml` の雛形を生成します。
新しいリポジトリやコンポーネントを Tazuna 管理に乗せる最初の一歩に向きます。

```text
tazuna init [-f tazuna.yaml] [--force]
```

## 振る舞い

1. 出力先（`-f` / `--file-path`、デフォルト `tazuna.yaml`）が既に存在し、かつ
   `--force` が無ければエラーで終了します（既存ファイルを誤って壊さないため）。
2. 次の内容で `tazuna.yaml` を書き出し、`created: <path>` を標準出力に書きます。
   - `apiVersion` / `kind` を正規値で設定。
   - `spec.minimumSupportedTazunaVersion` を、**生成した tazuna 自身のバージョン**に
     ピン留め。これにより、より古い tazuna バイナリがこのファイルを処理しようとすると
     エラーで止まります。
   - `spec.manifests` は空 (`[]`)。includes でコンポーネントごとの `tazuna.yaml` を
     読み込むためのコメント例を添えています。

生成される雛形（バージョンは生成時の tazuna に依存します）:

```yaml
apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  # この tazuna.yaml を処理するのに必要な tazuna の最小バージョン (semver) です。
  # これを下回る tazuna バイナリは、誤適用を防ぐためエラーで終了します。
  minimumSupportedTazunaVersion: "1.4.0"
  # includes で各コンポーネントの tazuna.yaml を読み込みます。
  # 下の例のように manifests に includes エントリを追加してください。
  #
  #   manifests:
  #     - name: infra
  #       includes:
  #         - path: ./infra/tazuna.yaml
  #         - path: ./addons/tazuna.yaml
  manifests: []
```

生成直後の雛形はそのまま [`tazuna check`](./check.md) を通過します。

## バージョンのピン留めについて

- 実行中の tazuna が semver なバージョン（リリースビルド）なら、その値を正規化して
  `minimumSupportedTazunaVersion` に埋め込みます（先頭の `v` は除去されます）。
- ローカルビルド（`dev` など semver でないバージョン）で実行した場合は、
  プレースホルダとして `0.0.0` を埋め込みます。リリース版で生成し直すか、手で
  適切なバージョンに書き換えてください。

`minimumSupportedTazunaVersion` の比較ルールそのものは
[`tazuna.yaml` スキーマ - `minimumSupportedTazunaVersion`](../tazuna-yaml.md#minimumsupportedtazunaversion)
を参照してください。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) に加えて次を受け付けます。

| フラグ    | エイリアス | 型   | デフォルト | 説明 |
|-----------|------------|------|------------|------|
| `--force` | -          | bool | `false`    | 出力先が既に存在していても上書きします。 |

引数は受け付けません。

## 例

```bash
tazuna init
tazuna init -f infra/tazuna.yaml
tazuna init --force
```

## 関連

- 生成されるフィールドの意味: [`tazuna.yaml` スキーマ](../tazuna-yaml.md)
- includes の使い方: [`tazuna.yaml` スキーマ - includes を使う](../tazuna-yaml.md#includes-を使う)
