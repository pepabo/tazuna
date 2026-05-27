# `type: kustomize`

`kustomize` Manifest は、kustomize でレンダリングした Kubernetes manifest をクラスタへ反映します。
Tazuna は内部で kustomize (`sigs.k8s.io/kustomize`) を呼び、`kustomize build <path>` 相当の
結果を生成して使います。

## `path`

`kustomization.yaml` が置かれているディレクトリを指します。
**`tazuna.yaml` 自身のディレクトリ起点** の相対パスで書きます。
ディレクトリ内に妥当な `kustomization.yaml` が無いと `tazuna build` / `apply` が失敗します。

## 固有フィールド

`manifests[].kustomize` のオブジェクトに書きます。

| フィールド          | 型     | 必須 | デフォルト | 説明 |
|---------------------|--------|------|------------|------|
| `defaultNamespace`  | string | -    | `""`       | レンダリング結果のリソースで `metadata.namespace` が未指定のものに付与する namespace。空のときはリソース側に書かれた namespace（無ければ Kubernetes 既定の `default`）が使われます。 |

## 振る舞い

| 操作      | 内部処理 |
|-----------|----------|
| `Build`   | `kustomize build <path>` 相当のレンダリングを行い、結果 YAML を標準出力に書く。 |
| `Apply`   | レンダリング結果を unstructured オブジェクト群に変換し、`defaultNamespace` を補完したうえで 1 つずつ `CreateOrUpdate` する。 |
| `Destroy` | レンダリング結果を unstructured オブジェクト群に変換し、`defaultNamespace` を補完したうえで 1 つずつ削除する。 |

`kustomize build` 自体はクラスタへの接続を必要としません。
ローカルファイルだけで完結します。

## 例

```yaml
manifests:
  - name: ingress-nginx
    type: kustomize
    path: ./kustomize/ingress-nginx
    tags:
      - infra

  - name: app-overlay
    type: kustomize
    path: ./kustomize/app/overlays/staging
    kustomize:
      defaultNamespace: staging
```

## 関連

- [`tazuna.yaml` - Manifest](../tazuna-yaml.md#manifest)
- 用語: [kustomize](../../concepts/glossary.md#kustomize)
