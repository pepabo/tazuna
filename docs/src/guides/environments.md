# 環境ごとに設定を切り替える

1 つの `tazuna.yaml` を staging / production など複数のクラスタに使い回したい、
という場面は珍しくありません。Tazuna は次の 2 つの仕組みでこれを支えます。

- **`{{ .Environment }}` テンプレート変数** — `tazuna.yaml` を Go template として
  描画し、`-e/--environment` で渡した環境名を値として埋め込みます。
- **`spec.environments`** — 環境ごとに `context_matches` を宣言し、`-e` で選んだ
  環境の値で「どのクラスタへの適用を許すか」を切り替えます。

このガイドでは、この 2 つを組み合わせて「環境ごとにオーバーレイを切り替えつつ、
誤ったクラスタへの適用を防ぐ」ところまでを通します。仕様の詳細は
[`tazuna.yaml` スキーマ - environments](../reference/tazuna-yaml.md#environments) /
[テンプレート変数](../reference/tazuna-yaml.md#テンプレート変数) を参照してください。

## 1. `{{ .Environment }}` を埋め込む

まず、環境名でマニフェストのパスを切り替えます。

```yaml
apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
    - name: app
      type: kustomize
      path: ./overlays/{{ .Environment }}
```

`-e staging` を渡すと `path` は `./overlays/staging` に、`-e production` を渡すと
`./overlays/production` に描画されます。

```console
$ tazuna build -e staging   # ./overlays/staging をレンダリング
$ tazuna build -e production # ./overlays/production をレンダリング
```

`-e` を渡さない場合、`{{ .Environment }}` は **空文字列** に展開されます
（この例では `path: ./overlays/` になります）。描画は `tazuna.yaml` を読み込む
すべてのコマンドで行われるため、`build` で結果を確認してから `apply` するとよいでしょう。

## 2. 環境ごとに適用先クラスタを限定する

テンプレート変数だけでは「production 用の設定を、間違えて staging クラスタに
適用してしまう」事故は防げません。そこで `spec.environments` で環境ごとに
`context_matches`（許可する kubeconfig context 名の正規表現）を宣言します。

```yaml
apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  environments:
    staging:
      context_matches:
        - ^staging-tokyo$
        - ^staging-osaka$
      context_match_mode: or
    production:
      context_matches:
        - ^prod-tokyo$
      context_match_mode: and
  manifests:
    - name: app
      type: kustomize
      path: ./overlays/{{ .Environment }}
```

- `tazuna apply -e staging` は、current-context が `staging-tokyo` または
  `staging-osaka` のときだけ実行されます。それ以外の context では中断します。
- `tazuna apply -e production` は、current-context が `prod-tokyo` のときだけ
  実行されます。

`-e` に対応する環境が `environments` に宣言されていない場合、
`apply` / `destroy` / `check` はエラーになります。タイプミスによる誤適用も防げます。

## 3. `check` で事前検証する

`tazuna check -e <name>` は、`tazuna.yaml` の描画・バリデーションに加えて、
指定した環境が `environments` に存在するかまで確認します。CI で
`tazuna check -e production` を回しておくと、production 適用前に設定ミスを検知できます。

```console
$ tazuna check -e production
$ tazuna check -e typo-env
error: environment "typo-env" is not declared under spec.environments
```

## `-e` を渡さないとき（ローカル開発）

`-e` を省略すると、`environments` は無視され、ルート直下の `context_matches` /
`context_match_mode` が使われます。ローカルの kind クラスタ向けの設定を
ルート直下に、staging / production を `environments` に置く、という使い分けが可能です。

```yaml
spec:
  context_matches:
    - ^kind-        # -e なしのローカル実行で使われる
  environments:
    staging:
      context_matches: [^staging-]
    production:
      context_matches: [^prod-]
  manifests:
    - name: app
      type: kustomize
      path: ./overlays/{{ .Environment }}
```

## まとめ

| やりたいこと                       | 使う仕組み |
|------------------------------------|------------|
| 環境名で値・パスを差し替える       | `{{ .Environment }}` |
| 環境ごとに適用先クラスタを限定する | `spec.environments[].context_matches` |
| 適用前に環境設定を検証する         | `tazuna check -e <name>` |
