# `tazuna.hint.yaml` スキーマ

`tazuna.hint.yaml` は、`type: helmfile` の Manifest で使う `vars` に対して
**「どんな値が取りうるか」「どれが必須か」** を宣言的に縛るためのヒントファイルです。
helmfile Manifest と同じディレクトリに置き、Tazuna が `vars` を解決する過程で参照します。

`tazuna.yaml` 本体のスキーマは [`tazuna.yaml` スキーマ](./tazuna-yaml.md) を参照してください。

## 置く場所と読み込み

Tazuna は **helmfile Manifest の `path` ディレクトリ直下** から `tazuna.hint.yaml` を探します。

- ファイル名は `tazuna.hint.yaml` 固定。
- 存在しなければそのまま無視されます（後方互換のため、エラーにはなりません）。
- 1 つの helmfile Manifest につき高々 1 つです。

helmfile 以外の Manifest type からは読まれません。

## ルート (`TazunaHint`)

| フィールド   | 型                                | 必須 | デフォルト | 説明 |
|--------------|-----------------------------------|------|------------|------|
| `apiVersion` | string                            | -    | -          | スキーマバージョンを示します。値は現状検証されません。 |
| `kind`       | string                            | -    | -          | リソース種別を示します。値は現状検証されません。 |
| `vars`       | map\<string, [HintVar](#hintvar)\> | ◯    | -          | `varName` をキーとする宣言の集合。 |
| `rules`      | [[HintRule](#hintrule)]           | -    | `[]`       | var 横断のトップレベルバリデーションルール。 |

`apiVersion` / `kind` は **値の検証は行われません** が、慣習として
`apiVersion: tazuna.pepabo.com/v1` / `kind: TazunaHint` を書いておくと、
後で検証が入った場合にも揃えやすくなります。

最小例:

```yaml
apiVersion: tazuna.pepabo.com/v1
kind: TazunaHint
vars:
  clusterName:
    type: string
    required: true
  replicas:
    type: string
    default: "3"
```

## HintVar

`vars` の各エントリ（var 1 つ分の宣言）です。

| フィールド          | 型       | 必須 | デフォルト | 説明 |
|---------------------|----------|------|------------|------|
| `type`              | string   | ◯    | -          | var の型。`string` / `slice` / `map` のいずれか。 |
| `required`          | bool     | -    | `false`    | ユーザーから必ず提供されなければならないかどうか。 |
| `default`           | any      | -    | `null`     | 未提供時に注入する値。`required: true` との併用不可。 |
| `description`       | string   | -    | `""`       | 人間向けの説明。挙動には影響しません。 |
| `format`            | string   | -    | `""`       | 値のフォーマット検証ルール。`type: string` のときだけ指定可能。詳細は [format](#format) 参照。 |
| `required_with`     | [string] | -    | `[]`       | 「ここで挙げた var のいずれかが提供されているとき、この var も必須」を意味する条件付き必須。`required: true` との併用不可。参照先は `vars` に存在しなければなりません。 |
| `required_without`  | [string] | -    | `[]`       | 「ここで挙げた var が **すべて** 未提供のとき、この var が必須」を意味する条件付き必須。`required: true` との併用不可。参照先は `vars` に存在しなければなりません。 |

### 値が未提供のときの振る舞い

helmfile Manifest 側の `vars` 解決後、各 var に対して次の順で処理されます。

1. すでに値が提供されていれば、そのまま採用する。
2. 提供されておらず `required: true` ならエラー。
3. `default` があればその値を注入する。
4. それ以外は **型ごとのゼロ値** を注入する（`string` → `""`、`slice` → `[]`、`map` → `{}`）。

条件付き必須（`required_with` / `required_without`）の判定は、上記のゼロ値注入の結果ではなく、
**ユーザーから明示的に提供された値の集合** に対して行われます。
ゼロ値が入った var が「提供済み」と誤判定されないようにするための実装です。

## format

`format` は `type: string` の var に対する文字列フォーマット検証です。
**値が空文字列（ゼロ値注入を含む）の場合は検証はスキップ** されます。
非空文字列が入っているときだけ検証が走ります。

| 値          | 検証 |
|-------------|------|
| `hostname`  | RFC 952 / 1123 に準拠したホスト名パターン（英数字 / `-` / `.`、各ラベルは英数字で開始・終了） |
| `url`       | `net/url.ParseRequestURI` で解釈でき、かつ scheme が空でないこと |
| `email`     | 簡易的な `user@domain.tld` 形式（簡易正規表現） |
| `ip`        | `net.ParseIP` で解釈できる IPv4 / IPv6 アドレス |
| `cidr`      | `net.ParseCIDR` で解釈できる CIDR 表記 |
| `uuid`      | RFC 4122 形式（ハイフン区切り） |
| `semver`    | セマンティックバージョン。`v` 接頭辞はオプション。pre-release / build metadata まで対応 |
| `datetime`  | `time.RFC3339` 形式の日時文字列 |

ここに無い値を指定するとバリデーションエラーになります。

## HintRule

`rules` は var 横断のバリデーションを宣言するためのトップレベルルールです。
個別 var の処理が一通り終わったあとに評価されます。

| フィールド  | 型       | 必須 | デフォルト | 説明 |
|-------------|----------|------|------------|------|
| `type`      | string   | ◯    | -          | ルール種別。現状は `oneof_required` のみ。 |
| `vars`      | [string] | ◯    | -          | ルールが対象とする var 名の配列。**2 件以上** が必要。参照先は `vars` に存在しなければなりません。 |
| `message`   | string   | -    | `""`       | バリデーションエラー時に表示するカスタムメッセージ。 |

### `oneof_required`

`vars` に挙げた var のうち、**少なくとも 1 つがユーザーから提供されていなければエラー**、というルールです。

```yaml
rules:
  - type: oneof_required
    vars:
      - certManagerIssuerName
      - certManagerClusterIssuerName
    message: "either certManagerIssuerName or certManagerClusterIssuerName must be set"
```

判定基準は条件付き必須と同じく、ゼロ値注入前の **ユーザー提供** の値の集合です。
`default` で値が入った var は「提供された」とは扱われません。

## バリデーション

`tazuna.hint.yaml` がロードされると、まずスキーマレベルで次の検証が行われます。

- `vars[*].type` が `string` / `slice` / `map` のいずれかであること。
- `vars[*].required: true` と `vars[*].default` を併用していないこと。
- `vars[*].format` を指定しているときに `type: string` であること。
- `vars[*].format` が既知の値であること（[format](#format) 参照）。
- `vars[*].required_with` / `vars[*].required_without` の参照先が `vars` に存在すること。
- `vars[*].required: true` と `required_with` / `required_without` を併用していないこと。
- `rules[*].type` が既知の値であること（現状は `oneof_required` のみ）。
- `rules[*].vars` の長さが 2 以上であること。
- `rules[*].vars` の参照先が `vars` に存在すること。

それらを通過したのち、helmfile Manifest の `vars` 解決結果に対して
次の検証が走ります（[値が未提供のときの振る舞い](#値が未提供のときの振る舞い) と
[format](#format) を参照）。

- hint で宣言された型と、helmfile 側に渡された値の型が整合すること
  （例: `type: slice` と宣言した var に `staticMap` が来たらエラー）。
- `required: true` の var に値が来ていること。
- `required_with` / `required_without` の条件を満たしていること。
- `format` を持つ string var の値が、空文字列でない場合に format パターンを満たしていること。
- `rules` を満たしていること（`oneof_required` の少なくとも 1 つが提供されていること）。

## 関連

- 用語: [`tazuna.hint.yaml`](../concepts/glossary.md#tazunahintyaml)
- helmfile Manifest の `vars` 側のスキーマ: [`tazuna.yaml` の Manifest type 別フィールド](./tazuna-yaml.md#manifest-type-別フィールド)
