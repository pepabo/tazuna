# `type: oras`

`oras` Manifest は、**OCI registry に置かれた artifact** を pull し、その内容を
`helmfile` または `kustomize` に **委譲して** クラスタへ反映する Manifest type です。

pull → 展開 → 委譲先 Manager の呼び出し、までを ORAS Manager がまとめて行います。
artifact の中身そのものは helmfile / kustomize と同じ作法で書きます。

## `path`

ORAS Manifest の `manifests[].path` は **実体としては使用されず、指定も不要** です。
他の Manifest type と異なり、ORAS では `path` を省略できます（書いても無視されます）。

委譲先 Manager に渡される `path` は、pull 後にローカルへ展開された
キャッシュディレクトリ（必要なら `target` でサブパスへ降りたもの）になります。

## 固有フィールド

`manifests[].oras` のオブジェクトに書きます。

| フィールド             | 型                                  | 必須 | デフォルト | 説明 |
|------------------------|-------------------------------------|------|------------|------|
| `reference`            | string                              | ◯    | -          | OCI artifact の reference。tag 形式 (`ghcr.io/example/foo:v1.0.0`) と digest 形式 (`ghcr.io/example/foo@sha256:...`) の両方を受け付けます。 |
| `target`               | string                              | -    | `""`       | 展開後の artifact root からの相対サブパス。省略時は root を指します。`..` などで artifact root を脱出する値は拒否されます。 |
| `plainHTTP`            | bool                                | -    | `false`    | `true` のとき registry への接続を HTTP（非 TLS）で行います。 |
| `insecureSkipVerify`   | bool                                | -    | `false`    | `true` のとき registry 接続時の TLS 証明書検証をスキップします。 |
| `auth`                 | [ORASAuth](#orasauth)               | -    | `null`     | registry の認証情報を override します。省略時は docker config.json を使用します。詳細は [認証の解決](#認証の解決) 参照。 |
| `delegate`             | [ORASDelegate](#orasdelegate)       | ◯    | -          | pull 後の委譲先 Manager の設定。 |

### ORASAuth

| フィールド   | 型     | 必須 | 説明 |
|--------------|--------|------|------|
| `username`   | string | -    | registry のユーザー名。 |
| `password`   | string | -    | registry のパスワード。 |

両フィールドが空のときは override 扱いになりません（[認証の解決](#認証の解決) 参照）。

### ORASDelegate

| フィールド   | 型                                | 必須 | デフォルト | 説明 |
|--------------|-----------------------------------|------|------------|------|
| `type`       | string                            | ◯    | -          | 委譲先の Manifest type。`helmfile` または `kustomize`。 |
| `helmfile`   | [ManifestHelmfile](./helmfile.md#固有フィールド)   | -    | `null`     | `type: helmfile` のときに委譲先へそのまま渡されるオプション。 |
| `kustomize`  | [ManifestKustomize](./kustomize.md#固有フィールド) | -    | `null`     | `type: kustomize` のときに委譲先へそのまま渡されるオプション。 |

## 振る舞い

| 操作      | 内部処理 |
|-----------|----------|
| `Build`   | artifact を pull → 委譲先の Build を呼ぶ。 |
| `Apply`   | artifact を pull → 委譲先の Apply を呼ぶ。 |
| `Destroy` | artifact を pull → 委譲先の Destroy を呼ぶ。 |

委譲先には次のように新しい `Manifest` が組み立てられて渡されます。

- `name` / `description` / `tags` / `tests` は元の ORAS Manifest からそのまま引き継ぐ。
- `type` は `delegate.type`（`helmfile` / `kustomize`）。
- `path` は pull 後のローカルパス（`target` 指定があれば追加でサブパスを結合）。
- 固有フィールドは `delegate.helmfile` / `delegate.kustomize` をそのまま使う。

`Test plugin` も同じ Manifest の文脈で評価されます。

### Pull とキャッシュ

ORAS の pull は **digest 単位でローカルにキャッシュ** されます。
同じ digest を持つ artifact の 2 回目以降の pull は registry にアクセスしません。

- キャッシュディレクトリ:
  - `$XDG_CACHE_HOME` が設定されていれば `$XDG_CACHE_HOME/tazuna/oras`
  - そうでなければ `$HOME/.cache/tazuna/oras`
- キャッシュ構造:
  - `blobs/<sanitized digest>/` 配下に artifact を展開する
  - `refs/<sanitized reference>` に tag → digest のマッピングを記録する
- `--no-cache` を `apply` / `build` / `destroy` に指定するとキャッシュを無視して常に registry から再取得します。
- `--offline` を指定すると registry へのアクセスを禁止します。キャッシュにヒットしなければエラーになります。
- `--no-cache` と `--offline` は同時には指定できません。
- CLI フラグは [`apply` / `build` / `destroy`](../cli/index.md#サブコマンド一覧) のページを参照。

### 展開時の制約

artifact 内の tar 展開には **ADR004 で定めた上限** が適用されます。

| 上限          | 値 |
|---------------|----|
| 展開後合計サイズ | 1 GiB |
| tar entry 数 | 10000 |

以下の不正な entry は拒否されます。

- 絶対パスを含む entry
- `..` で展開先ディレクトリを脱出する entry（zip slip）
- 展開先を脱出する symlink / hardlink
- サポート外の type（character device / block device / FIFO 等）

artifact の OCI manifest は **layer が 1 つだけ** であることが前提です。複数 layer の artifact は受け付けません。

### 認証の解決

registry に対する credential は次の優先順位で解決されます。

1. `oras.auth` の override（`username` / `password` の少なくとも一方が非空）
2. docker の credential store（`$DOCKER_CONFIG` または `~/.docker/config.json`）
3. anonymous（認証なし）

`oras.auth` を書いていても両フィールドが空のときは override 扱いされず、
docker 側へフォールバックします（意図しない anonymous 化を避けるため）。

同一プロセス内では token のキャッシュが共有されるため、同じ registry に対する複数の
pull で token を取り直すコストはかかりません。

### セキュリティ上の注意

ORAS artifact 内の helmfile / kustomize は **ローカルに書いた manifest と同じ信頼
レベルで実行されます**。artifact のレンダリング結果はそのままクラスタへ適用される
ため、信頼できる registry のみを参照し、tag ではなく **digest でのピン留め**
(`@sha256:...`) を推奨します。なお helmfile テンプレートでは sprig の
`env` / `expandenv` が無効化されており、artifact 側のテンプレートが tazuna
実行者の環境変数を読み取ることはできません。

## 例

tag 指定 + helmfile 委譲:

```yaml
manifests:
  - name: cert-manager
    type: oras
    # path は不要（ORAS では省略可能）
    oras:
      reference: ghcr.io/example/cert-manager-helmfile:v1.14.0
      delegate:
        type: helmfile
        helmfile:
          includeCRDs: true
          wait: true
```

digest 指定 + kustomize 委譲 + サブパス:

```yaml
manifests:
  - name: ingress-nginx
    type: oras
    oras:
      reference: registry.example.com/platform/ingress-bundle@sha256:abc123...
      target: kustomize/ingress-nginx
      auth:
        username: ci-bot
        password: ${REGISTRY_TOKEN}
      delegate:
        type: kustomize
        kustomize:
          defaultNamespace: ingress-nginx
```

## 関連

- 委譲先: [`type: helmfile`](./helmfile.md) / [`type: kustomize`](./kustomize.md)
- CLI: [`tazuna apply`](../cli/apply.md) / [`tazuna build`](../cli/build.md) / [`tazuna destroy`](../cli/destroy.md)
- 用語: [ORAS / OCI artifact](../../concepts/glossary.md#oras--oci-artifact)
