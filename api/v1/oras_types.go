package v1

// ORASDelegateType はORAS managerが委譲する先のmanager種別を表します
type ORASDelegateType string

const (
	// ORASDelegateTypeHelmfile はhelmfile managerに委譲します
	ORASDelegateTypeHelmfile ORASDelegateType = "helmfile"
	// ORASDelegateTypeKustomize はkustomize managerに委譲します
	ORASDelegateTypeKustomize ORASDelegateType = "kustomize"
)

// ManifestORAS はOCI registry (ORAS) からpullするmanifestの設定を表します。
// 詳細は docs/adr/004-oras-manager.md を参照してください。
type ManifestORAS struct {
	// Reference はOCI artifactのreferenceを指定します。
	// tag形式 (`ghcr.io/example/foo:v1.0.0`) と digest形式
	// (`ghcr.io/example/foo@sha256:...`) の両方を受け付けます。
	Reference string `json:"reference"`
	// Target はartifact展開後のルートからの相対サブパスを指定します。
	// 省略時はrootを指します。
	Target string `json:"target,omitempty"`
	// PlainHTTP はregistryへの接続にHTTP (非TLS) を使うかどうかを指定します。
	PlainHTTP bool `json:"plainHTTP,omitempty"`
	// InsecureSkipVerify はregistry接続時のTLS証明書検証をスキップします。
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// Auth はregistryの認証情報をoverrideします。省略時は docker config.json を使用します。
	Auth *ORASAuth `json:"auth,omitempty"`
	// Delegate はpull後の委譲先managerの設定を指定します。
	Delegate ORASDelegate `json:"delegate"`
}

// ORASAuth はORAS pull時の認証情報のoverrideを表します。
type ORASAuth struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// ORASDelegate はORAS managerが委譲する先のmanager設定を表します。
type ORASDelegate struct {
	// Type は委譲先managerの種別 (helmfile / kustomize) を指定します。
	Type ORASDelegateType `json:"type"`
	// Helmfile は Type が helmfile の場合に委譲先に渡す設定です。
	Helmfile *ManifestHelmfile `json:"helmfile,omitempty"`
	// Kustomize は Type が kustomize の場合に委譲先に渡す設定です。
	Kustomize *ManifestKustomize `json:"kustomize,omitempty"`
}
