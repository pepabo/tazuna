package v1

type GenesisSecret struct {
	Spec GenesisSecretSpec `yaml:"spec"`
}

type GenesisSecretSpec struct {
	Provider string                  `yaml:"provider"`
	Secrets  []GenesisSecretGenerate `yaml:"secrets"`
	Outputs  []GenesisSecretOutput   `yaml:"outputs"`
}

type GenesisSecretGenerate struct {
	// PreferLabelはID->ValueのマッピングではなくLabel->Valueを作る
	// カスタムのkey-valueを作るとIDがランダム文字列になるので、それを可能にするために定義する
	PreferLabel bool                                 `yaml:"preferLabel"`
	URI         string                               `yaml:"uri"`
	Items       map[string]GenesisSecretGenerateItem `yaml:"items"`
}

type GenesisSecretGenerateItem struct {
	MapTo string `yaml:"mapTo"`
}

type GenesisSecretOutput struct {
	Stdout           *GenesisSecretOutputStdout           `yaml:"stdout,omitempty"`
	KubernetesSecret *GenesisSecretOutputKubernetesSecret `yaml:"kubernetesSecret,omitempty"`
}

type GenesisSecretOutputStdout struct{}

type GenesisSecretOutputKubernetesSecret struct {
	Context     string            `yaml:"context"`
	Namespace   string            `yaml:"namespace"`
	Name        string            `yaml:"name"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
	// corev1.SecretType を指定する
	Type string `yaml:"type"`
}

func (GenesisSecret) GetKind() string {
	return "GenesisSecret"
}
func (GenesisSecret) GetName() string {
	return ""
}
