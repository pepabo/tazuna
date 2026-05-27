# `tazuna tags`

`tazuna.yaml` で宣言されているタグを一覧します。
タグごとに、そのタグが付いている Manifest の name を表示します。

```text
tazuna tags [-f tazuna.yaml] [--tags ...]
```

## 振る舞い

1. `tazuna.yaml` をロードしてバリデーションする。
2. `includes` 展開後の全 Manifest を走査し、`tags` を `タグ名 → Manifest 名のリスト`
   のマップに集約する。
3. タグ名でソートして標準出力に出力する。出力は次の形式です。

```text
<tag>:
- <manifest-name>
- <manifest-name>
```

4. `--tags` が指定された場合は、そのタグ名に絞って出力する。

クラスタへのアクセスはありません。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) に加えて次を受け付けます。

| フラグ    | エイリアス | 型       | デフォルト | 説明 |
|-----------|------------|----------|------------|------|
| `--tags`  | `-t`       | []string | `[]`       | 出力対象を指定したタグ名に絞り込みます。 |

## 例

```bash
tazuna tags
tazuna tags -f tazuna.yaml
tazuna tags --tags frontend,backend
```

## 関連

- `--tags` の絞り込み仕様: [`manifests[].tags`](../tazuna-yaml.md#tags)
- 絞り込んで apply する場合は [`tazuna apply`](./apply.md)
