package v1

type GenesisSecret struct {
	Spec GenesisSecretSpec `json:"spec"`
}

type GenesisSecretSpec struct {
	Provider string                  `json:"provider"`
	Secrets  []GenesisSecretGenerate `json:"secrets"`
	// Static は Provider を経由せず、この YAML に直接書いた固定値をそのまま
	// 出力 Secret の data に含めるためのマップです。キーがそのまま出力
	// Secret の data キー名になります（Provider 由来の secrets[].items のような
	// mapTo リネームは行いません）。
	// url や type のように、値自体は秘匿情報ではないが同じ Secret 内に
	// 含める必要があるフィールド向けです。
	Static  map[string]string     `json:"static,omitempty"`
	Outputs []GenesisSecretOutput `json:"outputs"`
}

type GenesisSecretGenerate struct {
	// PreferLabelはID->ValueのマッピングではなくLabel->Valueを作る
	// カスタムのkey-valueを作るとIDがランダム文字列になるので、それを可能にするために定義する
	PreferLabel bool                                 `json:"preferLabel"`
	URI         string                               `json:"uri"`
	Items       map[string]GenesisSecretGenerateItem `json:"items"`
}

type GenesisSecretGenerateItem struct {
	MapTo string `json:"mapTo"`
}

type GenesisSecretOutput struct {
	Stdout           *GenesisSecretOutputStdout           `json:"stdout,omitempty"`
	KubernetesSecret *GenesisSecretOutputKubernetesSecret `json:"kubernetesSecret,omitempty"`
}

type GenesisSecretOutputStdout struct{}

type GenesisSecretOutputKubernetesSecret struct {
	Context     string            `json:"context"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	// corev1.SecretType を指定する
	Type string `json:"type"`
}
