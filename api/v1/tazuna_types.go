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
	APIVersion string `yaml:"apiVersion,omitempty"`
	// Kind は Kubernetes manifest と同形式の TypeMeta フィールドです。
	// 設定する場合は TazunaKind と一致している必要があります。
	Kind string     `yaml:"kind,omitempty"`
	Spec TazunaSpec `yaml:"spec"`
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
	// ContextMatchesは現在のkubeconfigコンテキスト名がマッチすべき正規表現パターンのリストです
	// 指定した場合、apply/destroy時にコンテキスト名がパターンにマッチしないとエラーになります
	ContextMatches []string `yaml:"context_matches,omitempty"`
	// ContextMatchModeはcontext_matchesの評価モードです（"or" または "and"、デフォルトは "or"）
	ContextMatchMode ContextMatchMode `yaml:"context_match_mode,omitempty"`
	Manifests        []Manifest       `yaml:"manifests"`
	// Testsはすべてのマニフェスト適用が終わったあとに実行されます
	Tests []TestPluginSpec `yaml:"tests"`
}

// IncludeFile はincludeするファイルを定義します
type IncludeFile struct {
	// Path はincludeするファイルのパス（tazuna.yamlからの相対パス）
	Path string `yaml:"path"`
}

// Manifest はtazunaのマニフェスト管理方式を定義します
type Manifest struct {
	Name        string `yaml:"name,omitempty"`        // マニフェストの名前
	Description string `yaml:"description,omitempty"` // マニフェストの説明

	// Includes はincludeするファイルのリストを指定します
	// includesが指定された場合、他のフィールド（Type, Path, Tags など）は無視されます
	Includes []IncludeFile `yaml:"includes,omitempty"`

	// Path はマニフェストのパスを指定します
	// GenesisSecretの場合はGenesisSecretのリソースマニフェストのパス
	// Kustomizeの場合はkustomization.yamlがあるディレクトリ
	// Helmfileの場合はhelmfile.yamlがあるディレクトリ
	Path string       `yaml:"path"`
	Type ManifestType `yaml:"type"`
	// Tagsはマニフェストに付与するタグを指定します
	// タグはマニフェストの選択に利用できます
	// 例えば、`tazuna apply --tags foo,bar`とすると、fooとbarのタグが付与されたマニフェストのみが適用されます
	Tags          []string               `yaml:"tags,omitempty"`
	Kustomize     *ManifestKustomize     `yaml:"kustomize,omitempty"`
	GenesisSecret *ManifestGenesisSecret `yaml:"genesisSecret,omitempty"`
	Helmfile      *ManifestHelmfile      `yaml:"helmfile,omitempty"`
	Parallel      *ManifestParallel      `yaml:"parallel,omitempty"`
	ORAS          *ManifestORAS          `yaml:"oras,omitempty"`
	// Testsはマニフェストapply後に行われる各種テストを記載します
	Tests []TestPluginSpec `yaml:"tests"`
}

type ManifestType string

const (
	ManifestTypeKustomize     ManifestType = "kustomize"
	ManifestTypeGenesisSecret ManifestType = "genesissecret"
	ManifestTypeHelmfile      ManifestType = "helmfile"
	ManifestTypeParallel      ManifestType = "parallel"
	ManifestTypeORAS          ManifestType = "oras"
)

type ManifestKustomize struct {
	DefaultNamespace string `yaml:"defaultNamespace,omitempty"` // kustomize assetsのデフォルトネームスペース
}

type ManifestGenesisSecret struct{}
type ManifestHelmfile struct {
	IncludeCRDs      bool                   `yaml:"includeCRDs"`
	Vars             map[string]HelmFileVar `yaml:"vars,omitempty"`
	DefaultNamespace string                 `yaml:"defaultNamespace,omitempty"` // helmfile assetsのデフォルトネームスペース
	ExtraValueFiles  []string               `yaml:"extraValueFiles,omitempty"`  // 追加のvalue filesを指定
	Wait             bool                   `yaml:"wait,omitempty"`             // helmfile syncに--waitオプションを渡す
	TimeoutSeconds   int                    `yaml:"timeoutSeconds,omitempty"`   // リソースのReady待機のタイムアウト秒数
	KubeVersion      string                 `yaml:"kubeVersion,omitempty"`      // helm templateに渡す--kube-versionの値
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
	From        string                    `yaml:"from,omitempty"`
	Op          *OnePasswordVaultSelector `yaml:"op,omitempty"`
	Static      *string                   `yaml:"static,omitempty"`      // staticな値を指定する
	StaticSlice []string                  `yaml:"staticSlice,omitempty"` // staticなslice値を指定する
	StaticMap   map[string]string         `yaml:"staticMap,omitempty"`   // staticなmap値を指定する
	Env         *string                   `yaml:"env,omitempty"`         // 環境変数から値を取得する
}

type OnePasswordVaultSelector struct {
	Key   string `yaml:"key"`   // 1PasswordのFieldをIDかLabelのどちらから取ってくるか
	Vault string `yaml:"vault"` // 1PasswordのVault名
	Item  string `yaml:"item"`  // 1PasswordのItem名
	Field string `yaml:"field"` // 1PasswordのField名
}

type ManifestParallel struct {
	// Childrenはマニフェストの子マニフェストを定義します
	Children []Manifest `yaml:"children,omitempty"`
}
