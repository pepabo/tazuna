# `tazuna state sync`

各 Manager の Build 結果と State を比較し、追加・変更されたリソースだけを
クラスタへ反映します。同期に成功したリソースの State は ConfigMap に書き戻されます。

```text
tazuna state sync [-f tazuna.yaml] [--atomic]
```

## 振る舞い

1. `tazuna.yaml` をロードする。
2. 各 Manifest について Build を呼び出し、State との差分を計算する。
3. `added` / `modified` / `always-sync` 分類のリソースをクラスタへ反映する。
4. `removed` 分類のリソースは **デフォルトではスキップ** される。
   `TAZUNA_STATE_SYNC_DELETE=true` を設定したときに限り、削除を行う。
5. 同期に成功したリソースの State を書き戻す。

`--atomic` を指定した場合、いずれかのリソースでエラーが発生したときは
State をまったく更新せずに終了します（途中まで進んだ反映自体は巻き戻りません）。

`context_matches` の評価は行いません。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) に加えて次を受け付けます。

| フラグ       | エイリアス | 型   | デフォルト | 説明 |
|--------------|------------|------|------------|------|
| `--atomic`   | -          | bool | `false`    | エラーが発生したときに State を更新せずに終了します。 |

## 環境変数

| 環境変数                    | 値     | 説明 |
|-----------------------------|--------|------|
| `TAZUNA_STATE_SYNC_DELETE`  | `true` | `removed` 分類のリソースを削除します。設定されていない場合は削除を行いません。 |

## 例

```bash
tazuna state sync
tazuna state sync -f tazuna.yaml
tazuna state sync --atomic
TAZUNA_STATE_SYNC_DELETE=true tazuna state sync
```

## 関連

- 差分の確認は [`tazuna state diff`](./state-diff.md)
- 用語: [Diff type](../../concepts/glossary.md#diff-type) / [always-sync](../../concepts/glossary.md#always-sync)
