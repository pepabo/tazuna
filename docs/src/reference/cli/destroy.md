# `tazuna destroy`

Tazuna が管理しているリソースをクラスタから削除します。
事故を防ぐために **二段階のガード** が掛かっています。

```text
TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy [-f tazuna.yaml] \
  [--tags ...] [--force] [--no-cache | --offline]
```

## 振る舞い

1. `tazuna.yaml` をロードしてバリデーションする。
2. `spec.context_matches` が設定されていれば、current-context と照合する。
   合致しなければ即終了する。
3. `--force` が無ければ、次のプロンプトを出して Y/N の確認を取る。

   ```text
   !!! All resources managed by Tazuna will be deleted !!!
   Are you sure you want to delete them?
   ```
4. 環境変数 `TAZUNA_DESTROY_EXECUTABLE` が `true` でなければ、
   ログだけ出してクラスタには **触れず** に終了する。
5. ガードを通過した場合のみ、`--tags` フィルタを適用したうえで Manager の
   Destroy を順に呼び出し、対応リソースをクラスタから削除する。

つまり「プロンプトで Yes」「`TAZUNA_DESTROY_EXECUTABLE=true`」の **両方** を満たさない限り、
リソースは消えません。

## フラグ

[グローバルフラグ](./index.md#グローバルフラグ) に加えて次を受け付けます。

| フラグ        | エイリアス | 型       | デフォルト | 説明 |
|---------------|------------|----------|------------|------|
| `--force`     | -          | bool     | `false`    | 削除前の確認プロンプトをスキップします。**環境変数のガードはスキップしません。** |
| `--tags`      | `-t`       | []string | `[]`       | 指定したタグのいずれかが付いている Manifest だけを削除対象にします（OR 評価）。 |
| `--no-cache`  | -          | bool     | `false`    | `type: oras` の Manifest で、キャッシュを使わずに常に registry から再取得します。 |
| `--offline`   | -          | bool     | `false`    | `type: oras` の Manifest で、registry へのアクセスを禁止します。 |

`--no-cache` と `--offline` は同時に指定できません。

## 環境変数

| 環境変数                       | 値     | 説明 |
|--------------------------------|--------|------|
| `TAZUNA_DESTROY_EXECUTABLE`    | `true` | これが設定されていない限り、`destroy` は何も削除しません。CI で誤って destroy が走るのを防ぐためのキルスイッチです。 |

## 例

```bash
TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy
TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy --tags experimental
TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy --force
```

## 関連

- 評価される [`context_matches`](../tazuna-yaml.md#context_matches)
- 反映側は [`tazuna apply`](./apply.md)
