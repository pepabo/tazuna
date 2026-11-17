package v1

// TFStateBackendType は tfstate を読み出す backend の種類を表す識別子です。
// terraform の backend 名 (`backend "s3" {}` の "s3" 等) に揃えています。
type TFStateBackendType string

const (
	// TFStateBackendTypeS3 は AWS S3 (および S3 互換ストレージ) に置かれた tfstate を読みます。
	TFStateBackendTypeS3 TFStateBackendType = "s3"
	// TFStateBackendTypeLocal はローカルファイルシステム上の tfstate を読みます。
	TFStateBackendTypeLocal TFStateBackendType = "local"
)

// DefaultS3WorkspaceKeyPrefix は terraform s3 backend の workspace_key_prefix の
// デフォルト値です。
const DefaultS3WorkspaceKeyPrefix = "env:"

// TFStateSource は tazuna.yaml の spec.tfstates[] エントリで、名前付きの tfstate
// 参照先を 1 つ宣言します。helmfile vars (`from: tfstate`)、template 関数
// (`{{ tfstate "<name>" "<address>" }}`)、tfstate 型の Secret provider は
// いずれもこの Name で参照先を指定します。
type TFStateSource struct {
	// Name は tfstate ソースの識別子です。spec.tfstates[] 内で一意である必要があります。
	Name string `json:"name"`
	// Backend は tfstate を読み出す backend の設定です。
	Backend TFStateBackend `json:"backend"`
}

// TFStateBackend は tfstate を読み出す backend の設定です。Type に応じて
// 対応するサブフィールド (S3 / Local) のいずれか 1 つだけを指定します。
type TFStateBackend struct {
	// Type は backend の種類です。"s3" / "local" のいずれかを指定します。
	Type TFStateBackendType `json:"type"`
	// S3 は Type が "s3" のときの設定です。
	S3 *TFStateS3Backend `json:"s3,omitempty"`
	// Local は Type が "local" のときの設定です。
	Local *TFStateLocalBackend `json:"local,omitempty"`
}

// TFStateS3Backend は S3 backend の設定です。フィールドは terraform の
// `backend "s3"` ブロックの同名設定に対応します。認証情報は tazuna.yaml には
// 書かず、AWS SDK 標準の credential chain (AWS_PROFILE / 環境変数 / SSO /
// インスタンスロール等) で解決します。
type TFStateS3Backend struct {
	// Bucket は tfstate が置かれている S3 バケット名です (必須)。
	Bucket string `json:"bucket"`
	// Key は tfstate オブジェクトのキーです (必須)。workspace を使う場合も
	// terraform の backend 設定と同じく workspace を含まないキーを指定します。
	Key string `json:"key"`
	// Region はバケットのリージョンです。省略時は AWS SDK 標準の解決
	// (AWS_REGION / AWS_DEFAULT_REGION / profile の region) に従います。
	Region string `json:"region,omitempty"`
	// Profile は使用する AWS shared config の profile 名です。省略時は
	// AWS_PROFILE 環境変数、それもなければ default profile が使われます。
	Profile string `json:"profile,omitempty"`
	// Workspace は terraform workspace 名です。空または "default" のときは Key を
	// そのまま使い、それ以外は `<WorkspaceKeyPrefix>/<Workspace>/<Key>` を読みます。
	Workspace string `json:"workspace,omitempty"`
	// WorkspaceKeyPrefix は workspace 使用時のキー prefix です。省略時は
	// terraform と同じ "env:" です。
	WorkspaceKeyPrefix string `json:"workspaceKeyPrefix,omitempty"`
	// Endpoint は S3 API のエンドポイント URL を上書きします。S3 互換ストレージ
	// (MinIO 等) 向けです。
	Endpoint string `json:"endpoint,omitempty"`
	// UsePathStyle は path-style addressing (https://<endpoint>/<bucket>/<key>) を
	// 使うかどうかです。S3 互換ストレージで必要になることがあります。
	UsePathStyle bool `json:"usePathStyle,omitempty"`
}

// TFStateLocalBackend は local backend の設定です。
type TFStateLocalBackend struct {
	// Path は tfstate ファイルのパスです (必須)。相対パスは tazuna.yaml の
	// ディレクトリを基準に解決します。
	Path string `json:"path"`
}

// TFStateRef は tfstate 内の 1 つの値への参照です。
type TFStateRef struct {
	// Source は spec.tfstates[].name のいずれかです。
	Source string `json:"source"`
	// Address は tfstate 内の値を指すアドレスです。
	// 例: "output.vpc_id", "aws_vpc.main.id", "module.network.aws_subnet.private[0].id"
	Address string `json:"address"`
}

// TFStateProviderConfig は tfstate 型 Secret provider の設定です。
type TFStateProviderConfig struct {
	// Source は spec.tfstates[].name のいずれかです。GenesisSecret の
	// secrets[].items のキーがこの tfstate 内のアドレスとして解釈されます。
	Source string `json:"source"`
}
