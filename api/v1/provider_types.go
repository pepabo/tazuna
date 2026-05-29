package v1

// Provider は (legacy) provider 要件宣言です。tazuna check のための環境前提として
// 必要な CLI コマンドを列挙する用途で利用されています。Secret provider の宣言
// (ProviderConfig) とは別物として残しています。
type Provider struct {
	Spec ProviderSpec `json:"spec"`
}

type ProviderSpec struct {
	Requirements []ProviderRequirement `json:"requirements"`
}

type ProviderRequirement struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}

func (Provider) GetKind() string {
	return "Provider"
}
func (Provider) GetName() string {
	return "provider"
}

// ProviderType は Secret provider の種類を表す識別子です。
type ProviderType string

const (
	// ProviderTypeOnePassword represents the built-in 1Password backed Secret provider.
	ProviderTypeOnePassword ProviderType = "onepassword"
	// ProviderTypeEnvFile represents the Secret provider that reads from a .env file.
	ProviderTypeEnvFile ProviderType = "envfile"
)

// DefaultOnePasswordProviderName is the reserved name of the built-in 1Password
// provider that is always available without explicit declaration in tazuna.yaml.
// GenesisSecret manifests with empty .spec.provider fall back to this provider
// to preserve backward compatibility.
const DefaultOnePasswordProviderName = "default-op"

// ProviderConfig は tazuna.yaml の spec.providers[] エントリで、単一の Secret
// provider 宣言を表現します。GenesisSecret の .spec.provider フィールドが
// この Name を参照することで、複数 provider の中から呼び分けることができます。
type ProviderConfig struct {
	// Name は GenesisSecret の .spec.provider から参照される識別子です。
	// "default-op" は組み込みの 1Password provider 用に予約されています。
	Name string `json:"name"`
	// Type は provider の種類です。"onepassword" / "envfile" のいずれかを指定します。
	Type ProviderType `json:"type"`

	// OnePassword は Type が "onepassword" のときに使う追加設定です。現状は空でも
	// OK ですが、将来的な拡張余地のために存在しています。
	OnePassword *OnePasswordProviderConfig `json:"onepassword,omitempty"`
	// EnvFile は Type が "envfile" のときに使う追加設定です。Path に .env ファイル
	// (tazuna.yaml からの相対パス) を指定します。
	EnvFile *EnvFileProviderConfig `json:"envfile,omitempty"`
}

// OnePasswordProviderConfig は OnePassword provider 用の追加設定です。
// 現状は空ですが、vault のホワイトリストや prefer-cli 等の拡張余地のために
// 構造体を分離しています。
type OnePasswordProviderConfig struct{}

// EnvFileProviderConfig は .env ファイルから値を読む provider の設定です。
type EnvFileProviderConfig struct {
	// Path is the path to a dotenv style file. Relative paths are resolved
	// against the directory containing tazuna.yaml.
	Path string `json:"path"`
}
