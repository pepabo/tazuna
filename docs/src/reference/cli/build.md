# `tazuna build`

`tazuna.yaml` で宣言された Manifest 群をレンダリングし、結果を標準出力に書き出します。
**クラスタは変更しません。** apply 前のプレビューや、別ツールへのパイプ入力として使います。

```text
tazuna build [-f tazuna.yaml] [--tags ...] [--no-cache | --offline]
```

## 振る舞い

1. `tazuna.yaml` をロードしてバリデーションする。
2. `--tags` でフィルタする。
3. 各 Manifest を対応する Manager の Build に渡し、結果を連結して標準出力に書き出す。

`context_matches` の評価は行いません。
クラスタへの reach も Manager の Build 実装次第ですが、
組み込みの Manager は基本的に kubeconfig を要求しません
（ORAS の registry pull は別途ネットワークアクセスを行います）。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) に加えて次を受け付けます。

| フラグ        | エイリアス | 型       | デフォルト | 説明 |
|---------------|------------|----------|------------|------|
| `--tags`      | `-t`       | []string | `[]`       | 指定したタグのいずれかが付いている Manifest だけを処理対象にします（OR 評価）。 |
| `--no-cache`  | -          | bool     | `false`    | `type: oras` の Manifest で、キャッシュを使わずに常に registry から再取得します。 |
| `--offline`   | -          | bool     | `false`    | `type: oras` の Manifest で、registry へのアクセスを禁止します。キャッシュにヒットしなければエラーになります。 |

`--no-cache` と `--offline` は同時に指定できません。

## 例

```bash
tazuna build -f tazuna.yaml
tazuna build -f tazuna.yaml --tags web
tazuna build -f tazuna.yaml | kubectl diff -f -
```

## 関連

- 反映する場合は [`tazuna apply`](./apply.md)
- 差分は [`tazuna state diff`](./state-diff.md)
