package v1

// HintVarType はhint varsの型を表します
type HintVarType string

const (
	HintVarTypeString HintVarType = "string"
	HintVarTypeSlice  HintVarType = "slice"
	HintVarTypeMap    HintVarType = "map"
)

// HintFormat はstring型varのフォーマット検証ルールを表します。
// format検証は値が非空文字列の場合のみ実行されます（ゼロ値注入による空文字列はスキップ）。
//
// 対応フォーマットと検証基準:
//   - hostname: RFC 952/1123に準拠したホスト名（英数字、ハイフン、ドット）
//   - url: net/url.ParseRequestURIで解析可能かつschemeが存在する
//   - email: 簡易的な正規表現による検証（user@domain形式）
//   - ip: net.ParseIPで解析可能なIPv4/IPv6アドレス
//   - cidr: net.ParseCIDRで解析可能なCIDR表記
//   - uuid: RFC 4122形式のUUID（ハイフン区切り）
//   - semver: セマンティックバージョニング（v接頭辞はオプション）
//   - datetime: time.RFC3339形式の日時文字列
type HintFormat string

const (
	HintFormatHostname HintFormat = "hostname"
	HintFormatURL      HintFormat = "url"
	HintFormatEmail    HintFormat = "email"
	HintFormatIP       HintFormat = "ip"
	HintFormatCIDR     HintFormat = "cidr"
	HintFormatUUID     HintFormat = "uuid"
	HintFormatSemver   HintFormat = "semver"
	HintFormatDatetime HintFormat = "datetime"
)

// HintRuleType はトップレベルバリデーションルールの種別を表します。
type HintRuleType string

const (
	// HintRuleTypeOneofRequired は指定されたvarのうち少なくとも1つがユーザーから提供されることを要求します。
	// 「提供済み」の判定はユーザーから明示的に渡されたresolvedVarsの存在で行い、
	// ゼロ値注入後のresultは参照しません。
	HintRuleTypeOneofRequired HintRuleType = "oneof_required"
)

// HintRule はトップレベルのバリデーションルールを表します。
// MergeVarsWithHintの実行時に、個別varの検証後に評価されます。
type HintRule struct {
	// Type はルールの種別です。現在は "oneof_required" のみ対応。
	Type HintRuleType `yaml:"type" json:"type"`
	// Vars はルールの対象となるvar名のリストです。2件以上が必要です。
	Vars []string `yaml:"vars" json:"vars"`
	// Message はバリデーションエラー時に表示するカスタムメッセージです。
	Message string `yaml:"message,omitempty" json:"message,omitempty"`
}

// TazunaHint はtazuna.hint.yamlのルートリソースです
type TazunaHint struct {
	APIVersion string             `yaml:"apiVersion" json:"APIVersion"`
	Kind       string             `yaml:"kind" json:"Kind"`
	Vars       map[string]HintVar `yaml:"vars" json:"Vars"`
	// Rules はvar横断のトップレベルバリデーションルールです。
	Rules []HintRule `yaml:"rules,omitempty" json:"Rules,omitempty"`
}

// HintVar はhint varsの定義です
type HintVar struct {
	Type        HintVarType `yaml:"type" json:"type"`
	Required    bool        `yaml:"required" json:"required"`
	Default     any         `yaml:"default,omitempty" json:"default,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	// Format はstring型varに対するフォーマット検証ルールです。
	// string型以外のvarに指定するとValidateHintでエラーになります。
	// 値が空文字列（ゼロ値注入を含む）の場合、検証はスキップされます。
	Format HintFormat `yaml:"format,omitempty" json:"format,omitempty"`
	// RequiredWith は、指定されたvarのいずれかがユーザーから提供された場合に、
	// このvarも必須になることを示します。
	// required:trueとの併用は矛盾するためValidateHintでエラーになります。
	RequiredWith []string `yaml:"required_with,omitempty" json:"required_with,omitempty"`
	// RequiredWithout は、指定されたvarが全てユーザーから未提供の場合に、
	// このvarが必須になることを示します。
	// required:trueとの併用は矛盾するためValidateHintでエラーになります。
	RequiredWithout []string `yaml:"required_without,omitempty" json:"required_without,omitempty"`
}
