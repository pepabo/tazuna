package v1

type GenesisSecret struct {
	Spec GenesisSecretSpec `json:"spec"`
}

type GenesisSecretSpec struct {
	Provider string                  `json:"provider"`
	Secrets  []GenesisSecretGenerate `json:"secrets"`
	Outputs  []GenesisSecretOutput   `json:"outputs"`
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
