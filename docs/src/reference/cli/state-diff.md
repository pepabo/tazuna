# `tazuna state diff`

各 Manager の Build 結果と、クラスタに保存されている State を比較し、
リソース単位の差分を出力します。クラスタは変更しません。

```text
tazuna state diff [-f tazuna.yaml]
```

## 振る舞い

1. `tazuna.yaml` をロードする。
2. 各 Manifest について、Manager の Build を呼び出して
   「いま `tazuna.yaml` から生成されるべきリソース」を組み立てる。
3. クラスタ上の State と突き合わせて、リソース単位で次のいずれかに分類して出力する。

| Diff type     | 意味 |
|---------------|------|
| `added`       | Build 結果には存在し、State には存在しない |
| `modified`    | 両方に存在するが、content hash が異なる |
| `removed`     | State にあるが、Build 結果には存在しない |
| `always-sync` | 差分計算をスキップし、常に同期する扱いの分類。`type: genesissecret` 由来の Secret はここに入る |

`context_matches` の評価は行いません。
クラスタへの read アクセスのみを行い、何も変更しません。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) 以外に固有フラグはありません。

## 例

```bash
tazuna state diff
tazuna state diff -f tazuna.yaml
```

## 関連

- 反映は [`tazuna state sync`](./state-sync.md)
- 全件レンダリング結果が欲しいときは [`tazuna build`](./build.md)
- 用語: [Diff type](../../concepts/glossary.md#diff-type) / [ContentHash](../../concepts/glossary.md#contenthash)
