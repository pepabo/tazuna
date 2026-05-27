# `tazuna apply`

`tazuna.yaml` で宣言された Manifest 群をクラスタへ反映します。
Tazuna の中心となるコマンドです。

```text
tazuna apply [-f tazuna.yaml] [--tags ...] [--no-cache | --offline]
```

## 振る舞い

実行順序は次のとおりです。クラスタに触れるのは 5 以降です。

1. `tazuna.yaml` をロードしてバリデーションする。
2. `spec.context_matches` が設定されていれば、current-context と照合する。
   合致しなければ即終了する。
3. `--tags` でフィルタする。
4. `manifests[]` を **宣言順** に走査する。
5. 各 Manifest を対応する Manager に渡し、クラスタへ反映する。
6. 各 Manifest の `tests` を実行する。
7. すべての Manifest 適用後、`spec.tests`（全体 Tests）を実行する。

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
tazuna apply -f tazuna.yaml
tazuna apply -f tazuna.yaml --tags web,batch
tazuna apply -f tazuna.yaml --log-level debug
```

## 関連

- 評価される [`context_matches`](../tazuna-yaml.md#context_matches)
- フィルタの仕様は [`manifests[].tags`](../tazuna-yaml.md#tags)
- 反映前にレンダリングだけ確認したい場合は [`tazuna build`](./build.md)
- 既存リソースを取り外す場合は [`tazuna destroy`](./destroy.md)
