# 最初の tazuna.yaml を書く

このガイドは、これから Tazuna を導入する人が **最初の `tazuna.yaml`** を書き、
1 つの Kubernetes クラスタに 1 つのアドオンを入れるところまでを通すための手順です。

ここでは題材として、`kustomize` で書いた小さな Deployment を 1 つ、
Tazuna 経由でクラスタに入れます。実運用ではここに ingress-nginx や cert-manager、
ArgoCD などが並びますが、最初の 1 枚を理解するためには Manifest は 1 つあれば十分です。

このガイドを終えると、次の状態になっています。

- 自分のリポジトリに `tazuna.yaml` と kustomize ディレクトリが 1 セットある
- `tazuna check` でその `tazuna.yaml` が妥当だと確認できる
- `tazuna build` でクラスタに触れずに「何が入るか」を確認できる
- `tazuna apply` でその内容がクラスタに反映されている

## 前提

### Kubernetes クラスタを 1 つ用意する

Tazuna はクラスタの作成そのものは行いません。コントロールプレーンの準備は前提です。
試すだけであれば、手元で動かせる軽量なクラスタで十分です。

- 手元で完結させたい場合: [KinD](https://kind.sigs.k8s.io/) や
  [minikube](https://minikube.sigs.k8s.io/) で 1 ノードのクラスタを立てる
- リモートでまず触ってみたい場合: EKS / GKE / AKS などのマネージドクラスタ、
  または `kubeadm` で立てた検証用クラスタ

`kubectl get nodes` が `Ready` を返す状態になっていれば、どれを選んでも構いません。

### tazuna バイナリを用意する

リリースページからバイナリを取得して `PATH` に置くか、`go install` で導入します。
詳細は [はじめかた](../getting-started/index.md) を参照してください。

`tazuna version` が動けば準備完了です。

### kubeconfig の current-context を確認する

Tazuna は kubeconfig の現在の context が指すクラスタに対して動きます。
**入れたい先のクラスタに current-context が向いていること** を、手を動かす前に必ず確認します。

```bash
kubectl config current-context
kubectl get nodes
```

複数クラスタを行き来する環境では、これを取り違えるのが最も典型的な事故の入口です。
context を取り違えても気付けるようにする仕組みとして `context_matches` がありますが、
最初の 1 枚では使わずに進めます（後続のガイドで扱います）。

## 作るもの

このガイドでは、次のようなディレクトリ構成を作ります。

```text
my-cluster/
├── tazuna.yaml
└── kustomize/
    └── nginx/
        ├── kustomization.yaml
        └── deployment.yaml
```

`tazuna.yaml` が「何を入れるか」を宣言し、`kustomize/nginx/` がその実体です。
このガイドではこの 2 階層だけを覚えておけば十分です。

## 1. kustomize ディレクトリを作る

まず、Tazuna が呼び出す側、つまり kustomize で書いた「入れたいもの」を用意します。
ここでは練習用に、`nginx` という Deployment を 1 つだけ持つ最小構成にします。

`my-cluster/kustomize/nginx/kustomization.yaml`:

```yaml
resources:
  - deployment.yaml
```

`my-cluster/kustomize/nginx/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
  labels:
    app: nginx
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx:1.27-alpine
          ports:
            - containerPort: 80
```

この時点で、Tazuna を使わなくても `kubectl apply -k my-cluster/kustomize/nginx`
で同じものをクラスタに入れることができます。Tazuna はあくまでこれを
**宣言的に管理するための一段上の層** として乗せるだけ、と捉えておくと理解しやすくなります。

## 2. tazuna.yaml を書く

次に、Tazuna に対する「唯一の入力ファイル」である `tazuna.yaml` を書きます。

`my-cluster/tazuna.yaml`:

```yaml
apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
    - name: nginx
      type: kustomize
      path: ./kustomize/nginx
```

最初は次の 3 つを押さえれば動かせます。

- `name` — この Manifest の識別子。State やログでこの名前が使われます。
  半角英数字とハイフン・アンダースコアで付けます。
- `type` — Tazuna がどのバックエンドでこの Manifest を扱うか。
  ここでは `kustomize` を指定します。`helmfile` や `oras` といった選択肢もあります
  （[Manifest type](../concepts/glossary.md#manifest-type) 参照）。
- `path` — 実体のあるディレクトリ。**`tazuna.yaml` 自身の置かれているディレクトリ起点**
  の相対パスで書きます。

`path` の起点が「コマンドを実行した cwd」ではなく「`tazuna.yaml` 自身の場所」である点には注意してください。
リポジトリのどこから `tazuna` を呼び出しても結果が変わらないように、こちらに合わせています。

## 3. check で妥当性を確認する

ファイルを書き終えたら、まずクラスタには触れずに `tazuna check` で `tazuna.yaml` を検証します。

```bash
cd my-cluster
tazuna check
```

`ok` と出れば、次の検査をすべて通っています。

- YAML としてパースできる
- `name` が必須でユニーク、使える文字種に収まっている
- `path` の指す場所が実際に存在する

ここで失敗するものは、すべて **クラスタに触る前に** 落とせます。
CI で最初に通す対象として組み込みやすいコマンドでもあります。

## 4. build でクラスタに触らず中身を見る

次に、`tazuna build` を使って「`tazuna apply` したら何が入るか」を、
クラスタには一切触れずに stdout に書き出します。

```bash
tazuna build
```

`kustomize build` 相当の Kubernetes manifest が出力されます。
今回の例では先ほど書いた `Deployment/nginx` がそのまま出てくるはずです。

`build` はクラスタへの接続を必要としません。
**意図したものが本当に入ろうとしているか** をレビューしたい場面や、
レンダリング結果を別のツールに食わせたい場面で使います。

## 5. apply でクラスタに反映する

ここまで来れば、本番作業はあと 1 コマンドです。
current-context が入れたいクラスタを指していることをもう一度確認してから、
`tazuna apply` を実行します。

```bash
kubectl config current-context   # 入れたいクラスタを指していること
tazuna apply
```

Tazuna は次のことを順に行います。

1. `tazuna.yaml` をロードして検証する
2. `manifests[]` を宣言順に走査する
3. 各 Manifest を対応する [Manager](../concepts/glossary.md#manager)（ここでは kustomize Manager）に渡す
4. kustomize Manager が `path` をレンダリングし、結果をクラスタに反映する
5. 反映した結果を [State](../concepts/glossary.md#state) としてクラスタ内に記録する

State はクラスタ内 `tazuna` namespace の ConfigMap (`tazuna-state-<manifest-name>`)
に置かれます。`tazuna` namespace が無ければ自動的に作られるので、
事前に作っておく必要はありません。

## 反映されたことを確認する

普段どおり `kubectl` で見ても良いですし、Tazuna 側からも確認できます。

```bash
kubectl get deployment -n default nginx
tazuna state list
```

`tazuna state list` には、いま Tazuna が「自分が入れたもの」として記録している
リソースが一覧で出ます。今回のガイドでは `Deployment/nginx` 1 件が出ているはずです。
ここに出ているリソースは、後で `tazuna destroy` で安全に取り外せる対象でもあります。

## よくある落とし穴

最初の 1 枚を書くときによく踏むものを並べておきます。

- **`path` の起点を勘違いする** — `tazuna.yaml` 自身のディレクトリ起点です。
  `cd` した場所からの相対パスのつもりで書くと、CI とローカルで挙動が食い違う原因になります。
- **クラスタを取り違える** — `kubectl config current-context` の確認を
  apply の直前に必ず入れる運用にします。`context_matches` を導入すれば仕組みで防げますが、
  まずは目視確認を習慣にします。
- **kustomize 側のエラーを Tazuna のエラーだと思う** — `tazuna build` が失敗するときは、
  ほとんどの場合 kustomize 側のエラーがそのまま伝播してきています。
  `kustomize build ./path` を直接実行して切り分けると速いです。
- **手で `kubectl apply` したものを混ぜる** — 同じクラスタに対して Tazuna 経由と
  手作業を混在させると、State と実体がずれていきます。混ぜる場合は、
  あとから `tazuna state diff` で差分を見られることだけ覚えておきます。

## 次に進む

次のガイドでは、ここで作った `tazuna.yaml` に Manifest を増やし、
**複数のアドオンを順序付きでクラスタに入れる** ところを扱う予定です。
そこから先で `--tags` による絞り込み、`includes` による分割、`context_matches` による
誤クラスタ事故の防止といった話に段階的に踏み込んでいきます。
