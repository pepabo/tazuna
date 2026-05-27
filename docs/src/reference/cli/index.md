# CLI

このセクションは `tazuna` バイナリが提供するすべてのサブコマンドの仕様を、
1 コマンド 1 ページで網羅します。

ページは規約書として読まれることを想定しています。
コマンドの選び方や運用上の使い分けは [ガイド](../../guides/index.md) を、
そもそも各コマンドが何を解いているかは [概念](../../concepts/index.md) を参照してください。

## サブコマンド一覧

- [`tazuna apply`](./apply.md) — `tazuna.yaml` をクラスタに反映する
- [`tazuna build`](./build.md) — クラスタに触らずにレンダリング結果を出力する
- [`tazuna check`](./check.md) — `tazuna.yaml` の妥当性を検証する
- [`tazuna destroy`](./destroy.md) — Tazuna 管理リソースをクラスタから削除する
- [`tazuna state list`](./state-list.md) — State に記録されているリソースを一覧する
- [`tazuna state diff`](./state-diff.md) — Build 結果と State の差分を出す
- [`tazuna state sync`](./state-sync.md) — 差分を State 主導で反映する
- [`tazuna secret-to-genesissecret`](./secret-to-genesissecret.md) — 既存 Secret を 1Password と GenesisSecret に書き出す
- [`tazuna tags`](./tags.md) — `tazuna.yaml` に書かれているタグを一覧する
- [`tazuna version`](./version.md) — バージョン情報を出力する

## グローバルフラグ

すべてのサブコマンドが継承する persistent フラグです。

| フラグ            | エイリアス | 型     | デフォルト    | 説明 |
|-------------------|------------|--------|---------------|------|
| `--file-path`     | `-f`       | string | `tazuna.yaml` | `tazuna.yaml` のパス。 |
| `--log-level`     | `-l`       | string | `info`        | ログレベル。`debug` / `info` / `warn` / `error` のいずれか。 |
| `--version`       | -          | -      | -             | ルートコマンドにのみ付くフラグ。バージョン情報を出して終了する。`tazuna version` と等価。 |

## 共通の振る舞い

### kubeconfig

クラスタアクセスを行うサブコマンドは、起動時に kubeconfig をロードし、
**current-context が指すクラスタ** に対して動作します。
`KUBECONFIG` 環境変数や `--kubeconfig` 相当のフラグは Tazuna 側では持たず、
kubectl と同じ解決ルールに従います。

### `context_matches` の評価

`tazuna.yaml` の `spec.context_matches` が設定されている場合、
**クラスタに触る直前** に current-context 名と照合します。

- 評価が走るコマンド: `apply` / `destroy`
- 評価が走らないコマンド: `build` / `check` / `state list` / `state diff` /
  `state sync` / `tags` / `version` / `secret-to-genesissecret`

評価モードは `spec.context_match_mode`（`or` / `and`、デフォルト `or`）に従います。
詳細は [`tazuna.yaml` スキーマ - context_matches](../tazuna-yaml.md#context_matches) を参照。

### tazuna.yaml のバリデーション

`apply` / `build` / `destroy` / `check` / `tags` は、いずれも実行の最初に
`tazuna.yaml` をロードしてバリデーションします。
バリデーションで失敗するとクラスタには触れません。
チェック項目の一覧は [`tazuna.yaml` スキーマ - バリデーションのまとめ](../tazuna-yaml.md#バリデーションのまとめ) を参照。

### 終了コード

| 終了コード | 意味 |
|------------|------|
| `0`        | 成功 |
| 非ゼロ     | 失敗。標準エラーに `error: ...` 形式でエラーが出力されます。 |

非ゼロ終了は CI でそのまま失敗扱いにできます。
コマンドごとに異なる終了コードを返すような区別は現状ありません。

## 環境変数

CLI フラグに加えて、Tazuna が参照する環境変数を一覧にしておきます。

| 環境変数                       | 値          | 影響するコマンド   | 効果 |
|--------------------------------|-------------|--------------------|------|
| `TAZUNA_DESTROY_EXECUTABLE`    | `true`      | `destroy`          | これが `true` に設定されていない限り、`destroy` は実体の削除を行いません。プロンプトで Yes と答えても、この環境変数が無ければ何も起こりません。 |
| `TAZUNA_STATE_SYNC_DELETE`     | `true`      | `state sync`       | これが `true` のときだけ、State には残っていてクラスタには無くなったリソース（removed）を削除します。デフォルトは削除をスキップします。 |
| `KUBECONFIG`                   | パス        | クラスタを触る全コマンド | 通常の kubectl と同じ kubeconfig 解決ルールに従います。 |
