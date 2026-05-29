# Secret provider

Secret provider は、`type: genesissecret` の Manifest が **どこから秘匿情報を取得するか**
を抽象化したものです。
`tazuna.yaml` の `spec.providers[]` で provider を宣言し、各 GenesisSecret YAML の
`spec.provider` でその名前を指定する形で対応付けます。

このページでは provider レジストリの仕組みと、`onepassword` / `envfile` の 2 つの
組み込み provider の宣言方法をまとめます。

## レジストリの基本

Runner は起動時に **provider レジストリ** を組み立てます。レジストリには次の 2 種類の
エントリが入ります。

- `tazuna.yaml` の `spec.providers[]` で宣言された各 provider
- 組み込みの **`default-op`**（1Password 向け）

`default-op` は常にレジストリに登録されており、宣言する必要はありません。
GenesisSecret の `spec.provider` が空文字だった場合の **後方互換フォールバック** として、
この `default-op` が選ばれます。

GenesisSecret の `spec.provider` に名前を書くと、その名前で provider レジストリから
provider を取り出して使います。

```yaml
# GenesisSecret YAML
apiVersion: tazuna.pepabo.com/v1
kind: GenesisSecret
spec:
  provider: ops-envfile   # ← tazuna.yaml の spec.providers[].name と一致させる
  secrets:
    - uri: env://ignored
      items:
        DATABASE_URL:
          mapTo: DATABASE_URL
  outputs:
    - kubernetesSecret:
        namespace: default
        name: app-config
```

## `spec.providers[]` の宣言

`tazuna.yaml` の `spec.providers[]` に書きます。

```yaml
# tazuna.yaml
spec:
  providers:
    - name: primary-op
      type: onepassword
      onepassword: {}
    - name: ops-envfile
      type: envfile
      envfile:
        path: ./secrets/ops.env
  manifests:
    - name: app-config
      type: genesissecret
      path: ./genesissecrets/app-config.yaml
```

| フィールド   | 型       | 必須 | デフォルト | 説明 |
|--------------|----------|------|------------|------|
| `name`       | string   | ◯    | -          | GenesisSecret の `spec.provider` から参照される名前。`default-op` は予約済みで使用不可。 |
| `type`       | string   | ◯    | -          | provider の種類。現在は `onepassword` または `envfile`。 |
| `onepassword`| object   | △    | `null`     | `type: onepassword` のときに使う追加設定。 |
| `envfile`    | object   | △    | `null`     | `type: envfile` のときに使う追加設定。 |

`type` と整合しない config（例: `type: onepassword` なのに `envfile:` が書かれている）は
バリデーションで弾かれます。

## `type: onepassword`

1Password アイテムから取得する provider です。組み込み実装は `op` CLI を呼び出して
値を取り出します。`onepassword` 追加設定は現状空オブジェクトで構いません
（将来の拡張余地のためにフィールドが分かれているだけです）。

```yaml
spec:
  providers:
    - name: primary-op
      type: onepassword
      onepassword: {}
```

GenesisSecret 側では `spec.secrets[].uri` を `op://<host>/<vault>/<item>` 形式で
書きます。詳細は [GenesisSecret スキーマ - `uri` の形式](./genesis-secret.md#uri-の形式)
を参照してください。

## `type: envfile`

dotenv 形式 (`KEY=VALUE` 1 行 1 ペア) のローカルファイルから値を読む provider です。
1Password に依存しない単体テストや、CI でローカル secret を流し込みたいケース、
ブートストラップ直前で 1Password の認証が用意できていない状況などで使えます。

```yaml
spec:
  providers:
    - name: ops-envfile
      type: envfile
      envfile:
        path: ./secrets/ops.env
```

| フィールド | 型     | 必須 | デフォルト | 説明 |
|-----------|--------|------|------------|------|
| `path`    | string | ◯    | -          | dotenv ファイルのパス。**`tazuna.yaml` 自身のディレクトリ起点** の相対パスとして解決されます。 |

`./secrets/ops.env` の中身は次のように、1 行 1 ペアです。

```text
DATABASE_URL=postgres://localhost/app
API_TOKEN=ghp_xxx
```

GenesisSecret 側の `spec.secrets[].uri` は使われません（envfile は単一ファイルの
key-value をそのまま返すため）。`items` のキーは envfile 上の key 名と一致させます。

## `spec.provider` 解決の流れ

GenesisSecret の `spec.provider` 値の扱いは次のとおりです。

| `spec.provider` の値 | 解決される provider |
|----------------------|---------------------|
| `""`（空文字 / 未設定） | 組み込みの `default-op`（1Password） |
| `"default-op"`        | 組み込みの `default-op` |
| その他の名前           | `tazuna.yaml` の `spec.providers[]` から同名のエントリを引く |

参照先の名前が `spec.providers[]` に存在しなければ apply 時にエラーになります。
`tazuna check` の段階でも、未定義名の参照は将来的にバリデーションで弾かれます。

## バリデーション

`tazuna check` および各コマンドの起動時、`spec.providers[]` は次のチェックを通ります。

- 各 `name` がユニークであること
- `name` が空文字でないこと
- `name` が **`default-op`**（予約名）でないこと
- `type` が `onepassword` / `envfile` のいずれかであること
- `type` と整合しない config（`type: onepassword` に `envfile:` が付いている等）が無いこと
- `type: envfile` のときは `envfile.path` が必須

## 関連

- GenesisSecret 全体: [GenesisSecret スキーマ](./genesis-secret.md)
- スキーマ側エントリ: [`tazuna.yaml` - Providers](./tazuna-yaml.md#providers)
- 用語: [Provider (SecretProvider)](../concepts/glossary.md#provider-secretprovider)
