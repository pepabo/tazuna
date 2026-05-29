# `tazuna state list`

クラスタに保存されている Tazuna の State を読み、
Tazuna 管理下にあるリソースとその content hash を一覧します。

```text
tazuna state list [-f tazuna.yaml]
```

## 振る舞い

1. `tazuna.yaml` をロードする。
2. 各 Manifest の `name` から対応する State ConfigMap
   （`tazuna` namespace の `tazuna-state-<manifest-name>`）を読む。
3. State に記録されている各リソースの GVK / namespace / name / content hash を
   標準出力に整形して出力する。

`context_matches` の評価は行いません。
クラスタへの read アクセスのみを行い、State を含むいかなるリソースも変更しません。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) 以外に固有フラグはありません。

## 例

```bash
tazuna state list
tazuna state list -f tazuna.yaml
```

## 関連

- 宣言 vs State の差分は [`tazuna state diff`](./state-diff.md)
- ライブクラスタとの drift は [`tazuna state drift`](./state-drift.md)
- 差分の反映は [`tazuna apply --sync`](./apply.md#state-連携---sync----prune----atomic)
- managed リソースの readiness は [`tazuna status`](./status.md)
- State の語彙は [用語集 - State](../../concepts/glossary.md#state)
