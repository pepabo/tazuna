# `dependsOn` による DAG 実行

Tazuna は、`tazuna.yaml` の `manifests[]` を **宣言順に逐次実行する** モードと、
`dependsOn` を使った **軽量 DAG モード** の 2 つの実行モデルを持っています。
このページでは、両者がどう切り替わり、DAG モードでどんな順序保証と並列性が
提供されるのかをまとめます。

## 切り替えの規則

`tazuna.yaml` の **どの Manifest にも `dependsOn` が書かれていない** 場合、
従来通り宣言順に 1 つずつ apply されます。これは後方互換のための既定挙動です。

```yaml
spec:
  manifests:
    - name: cni
      type: kustomize
      path: ./cni
    - name: ingress
      type: helmfile
      path: ./ingress
    - name: app
      type: kustomize
      path: ./app
```

このとき Tazuna は `cni` → `ingress` → `app` の順で 1 つずつ apply します。

**いずれか 1 つでも** `dependsOn` を持つ Manifest があれば、Runner は DAG モードに
切り替わります。

```yaml
spec:
  manifests:
    - name: cni
      type: kustomize
      path: ./cni
    - name: cert-manager
      type: helmfile
      path: ./cert-manager
      dependsOn: [cni]
    - name: ingress
      type: helmfile
      path: ./ingress
      dependsOn: [cni]
    - name: app
      type: kustomize
      path: ./app
      dependsOn: [cert-manager, ingress]
```

この場合、Runner は次の層を構築します。

| 層    | Manifest                       | 実行           |
|-------|--------------------------------|----------------|
| 第 1 層 | `cni`                          | 1 件のみ実行    |
| 第 2 層 | `cert-manager` / `ingress`     | 並列実行        |
| 第 3 層 | `app`                          | 1 件のみ実行    |

## トポロジカルソート

DAG モードでは Kahn のアルゴリズムでトポロジカルソートします。各イテレーションで
入次数が 0 のノード集合を「ひとつの層」として取り出し、その層が終わってから
依存していたノードの入次数を下げる、というのを繰り返します。

層内のマニフェストは **元の宣言順** で出力されるように安定ソートされるため、
同じ `tazuna.yaml` を何度かけても出力順は揺れません（並列実行自体は順不同です）。

## 同層内の並列実行

同じ層に入った Manifest は **goroutine で並列** に apply されます。
層単位での待ち合わせがあるため、たとえば `cert-manager` を待ってから `app` を入れる、
といった順序保証は崩れません。

層内で `apply` が失敗した場合、その層の他の goroutine の終了を待ってからエラーが
集約されます。途中キャンセルは現状行いません。

## バリデーション

`tazuna check` および `tazuna apply` 時、`dependsOn` は次のチェックを通ります。

- 列挙された Manifest 名が `includes` 展開後の全 Manifest 集合に存在すること
- 自分自身を `dependsOn` に含めていないこと
- 依存関係に **循環がない** こと

いずれかに違反していると、クラスタには一切触れずにエラー終了します。

## なぜ `parallel` ではなく `dependsOn` なのか

以前の Tazuna には `type: parallel` という Manifest type がありました。
これは「子 Manifest を並列に実行する箱」でしたが、次の問題がありました。

- 依存関係を `parallel` のネスト構造で表現するしかなく、複雑なケースが書きづらい
- `parallel` 配下の Manifest が状態管理（State / drift 検知）の枠から外れていた
- 「順序依存」「並列性」「グルーピング」が 1 つのフィールドに混ざっていた

`dependsOn` は「**ノード**（Manifest）と **エッジ**（依存）」というグラフの言葉で
これを書き直したものです。並列化は依存グラフから自動導出されるので、ユーザは
「順序」だけを宣言すればよく、並列化はランタイムに任せられます。
`type: parallel` は本リファクタで削除されています。

## ベストプラクティス

- 依存関係は「**前提となるリソースが Ready になっていてほしい Manifest**」にだけ書く。
  単にグルーピングしたいだけなら `tags` の方が適切。
- 長いチェーンを書く前に、「実は CRD 待ちなので [Test plugin の `WaitUntil`](../reference/test-plugin.md)
  でも代替できないか」を一度検討する。
- 循環依存を避けるには、`dependsOn` を **下位レイヤから上位レイヤへの一方向** だけに
  使うのがコツ（CNI → cert-manager → ingress → app）。

## 関連

- 全体アーキテクチャ: [全体アーキテクチャ](./architecture.md)
- スキーマ: [`tazuna.yaml` - Manifest](../reference/tazuna-yaml.md#manifest)
- 適用そのもの: [`tazuna apply`](../reference/cli/apply.md)
