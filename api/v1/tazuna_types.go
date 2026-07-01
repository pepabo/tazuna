package v1

const (
	// TazunaAPIVersion は Tazuna リソースが取りうる apiVersion の正規値です。
	TazunaAPIVersion = "tazuna.pepabo.com/v1"
	// TazunaKind は Tazuna リソースが取りうる kind の正規値です。
	TazunaKind = "Tazuna"
)

// Tazuna はtazuna applyの挙動を制御するルートリソースです
type Tazuna struct {
	// APIVersion は Kubernetes manifest と同形式の TypeMeta フィールドです。
	// 設定する場合は TazunaAPIVersion と一致している必要があります。
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind は Kubernetes manifest と同形式の TypeMeta フィールドです。
	// 設定する場合は TazunaKind と一致している必要があります。
	Kind string     `json:"kind,omitempty"`
	Spec TazunaSpec `json:"spec"`
}

// ContextMatchMode はcontext_matchesの評価モードを定義します
type ContextMatchMode string

const (
	// ContextMatchModeOR はいずれかのパターンにマッチすればOK（デフォルト）
	ContextMatchModeOR ContextMatchMode = "or"
	// ContextMatchModeAND はすべてのパターンにマッチする必要がある
	ContextMatchModeAND ContextMatchMode = "and"
)

type TazunaSpec struct {
	// MinimumSupportedTazunaVersion はこの tazuna.yaml を処理するのに必要な
	// tazuna バイナリの最小バージョンを semver 形式で宣言します（例: "1.4.0"）。
	// 指定した場合、tazuna.yaml を読み込んだ任意の操作で、実行中の tazuna の
	// バージョンがこの値を下回るとエラーで終了します。未指定なら制約はありません。
	// 先頭の "v" は許容されます（"v1.4.0" も可）。
	MinimumSupportedTazunaVersion string `json:"minimumSupportedTazunaVersion,omitempty"`
	// ContextMatchesは現在のkubeconfigコンテキスト名がマッチすべき正規表現パターンのリストです
	// 指定した場合、apply/destroy時にコンテキスト名がパターンにマッチしないとエラーになります
	ContextMatches []string `json:"context_matches,omitempty"`
	// ContextMatchModeはcontext_matchesの評価モードです（"or" または "and"、デフォルトは "or"）
	ContextMatchMode ContextMatchMode `json:"context_match_mode,omitempty"`
	// Environments は環境ごとの設定を宣言するマップです。キーが環境名になります。
	// `-e/--environment <name>` が渡されたとき、ルート直下の context_matches /
	// context_match_mode ではなく、ここで宣言した環境固有の値が使われます。
	// 未指定または `-e` が渡されない場合はルート直下の設定がそのまま使われます。
	Environments map[string]EnvironmentSpec `json:"environments,omitempty"`
	Manifests    []Manifest                 `json:"manifests"`
	// Testsはすべてのマニフェスト適用が終わったあとに実行されます
	Tests []TestPluginSpec `json:"tests"`
	// Providers は Secret provider の宣言リストです。未指定時は組み込みの "default-op"
	// (1Password) のみが利用可能であり、これは GenesisSecret の .spec.provider が空文字
	// だった場合の後方互換フォールバックとして利用されます。
	Providers []ProviderConfig `json:"providers,omitempty"`
}

// EnvironmentSpec は 1 つの環境 (`environments.<name>`) の設定を定義します。
// `-e/--environment <name>` が渡されたとき、ルート直下の同名フィールドの代わりに
// ここで宣言した値が使われます。
type EnvironmentSpec struct {
	// ContextMatches はこの環境で有効にする context_matches パターンのリストです。
	// ルート直下の context_matches を完全に置き換えます (マージはしません)。
	ContextMatches []string `json:"context_matches,omitempty"`
	// ContextMatchMode はこの環境における context_matches の評価モードです。
	// 空の場合はルート直下の context_match_mode を継承し、それも空なら "or" になります。
	ContextMatchMode ContextMatchMode `json:"context_match_mode,omitempty"`
}

// IncludeFile はincludeするファイルを定義します
type IncludeFile struct {
	// Path はincludeするファイルのパス（tazuna.yamlからの相対パス）
	Path string `json:"path"`
}

// Manifest はtazunaのマニフェスト管理方式を定義します
type Manifest struct {
	Name        string `json:"name,omitempty"`        // マニフェストの名前
	Description string `json:"description,omitempty"` // マニフェストの説明

	// Includes はincludeするファイルのリストを指定します
	// includesが指定された場合、他のフィールド（Type, Path, Tags など）は無視されます
	Includes []IncludeFile `json:"includes,omitempty"`

	// Path はマニフェストのパスを指定します
	// GenesisSecretの場合はGenesisSecretのリソースマニフェストのパス
	// Kustomizeの場合はkustomization.yamlがあるディレクトリ
	// Helmfileの場合はhelmfile.yamlがあるディレクトリ
	Path string       `json:"path"`
	Type ManifestType `json:"type"`
	// Tagsはマニフェストに付与するタグを指定します
	// タグはマニフェストの選択に利用できます
	// 例えば、`tazuna apply --tags foo,bar`とすると、fooとbarのタグが付与されたマニフェストのみが適用されます
	Tags          []string               `json:"tags,omitempty"`
	Kustomize     *ManifestKustomize     `json:"kustomize,omitempty"`
	GenesisSecret *ManifestGenesisSecret `json:"genesisSecret,omitempty"`
	Helmfile      *ManifestHelmfile      `json:"helmfile,omitempty"`
	ORAS          *ManifestORAS          `json:"oras,omitempty"`
	// DependsOn はこのマニフェスト適用前に完了している必要があるマニフェスト名のリスト。
	// dependsOn に列挙された全マニフェストが apply 成功した後でのみ、このマニフェストの
	// apply が開始される。同じ層に属するマニフェスト (依存関係上同じ深度) は並列に
	// 適用される。dependsOn が tazuna.yaml 内で一度も使われていなければ従来通りの
	// 宣言順・順次実行となるため後方互換性が保たれる。
	// 自分自身の参照、未知のマニフェスト名、循環依存はいずれもバリデーションエラー。
	DependsOn []string `json:"dependsOn,omitempty"`
	// Testsはマニフェストapply後に行われる各種テストを記載します
	Tests []TestPluginSpec `json:"tests"`
}

type ManifestType string

const (
	ManifestTypeKustomize     ManifestType = "kustomize"
	ManifestTypeGenesisSecret ManifestType = "genesissecret"
	ManifestTypeHelmfile      ManifestType = "helmfile"
	ManifestTypeORAS          ManifestType = "oras"
)

type ManifestKustomize struct {
	DefaultNamespace string `json:"defaultNamespace,omitempty"` // kustomize assetsのデフォルトネームスペース
}

type ManifestGenesisSecret struct{}
type ManifestHelmfile struct {
	IncludeCRDs      bool                   `json:"includeCRDs"`
	Vars             map[string]HelmFileVar `json:"vars,omitempty"`
	DefaultNamespace string                 `json:"defaultNamespace,omitempty"` // helmfile assetsのデフォルトネームスペース
	ExtraValueFiles  []string               `json:"extraValueFiles,omitempty"`  // 追加のvalue filesを指定
	Wait             bool                   `json:"wait,omitempty"`             // helmfile syncに--waitオプションを渡す
	TimeoutSeconds   int                    `json:"timeoutSeconds,omitempty"`   // リソースのReady待機のタイムアウト秒数
	KubeVersion      string                 `json:"kubeVersion,omitempty"`      // helm templateに渡す--kube-versionの値
}

func DefaultHelmfile() *ManifestHelmfile {
	return &ManifestHelmfile{
		IncludeCRDs: false,
		Vars:        make(map[string]HelmFileVar),
	}
}

const (
	HelmFileVarFromEnv    = "env"
	HelmFileVarFromStatic = "static"
	HelmFileVarFromOp     = "op"

	HelmFileVarOpKeyID    = "id"
	HelmFileVarOpKeyLabel = "label"
)

type HelmFileVar struct {
	// Fromはどこからhelmfile varの値を取得するかを指定します。
	// 現状は `env` と `op`, `static` をサポートしています
	From        string                    `json:"from,omitempty"`
	Op          *OnePasswordVaultSelector `json:"op,omitempty"`
	Static      *string                   `json:"static,omitempty"`      // staticな値を指定する
	StaticSlice []string                  `json:"staticSlice,omitempty"` // staticなslice値を指定する
	StaticMap   map[string]string         `json:"staticMap,omitempty"`   // staticなmap値を指定する
	Env         *string                   `json:"env,omitempty"`         // 環境変数から値を取得する
}

type OnePasswordVaultSelector struct {
	Key   string `json:"key"`   // 1PasswordのFieldをIDかLabelのどちらから取ってくるか
	Vault string `json:"vault"` // 1PasswordのVault名
	Item  string `json:"item"`  // 1PasswordのItem名
	Field string `json:"field"` // 1PasswordのField名
}
