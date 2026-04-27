package v1

type Provider struct {
	Spec ProviderSpec `yaml:"spec"`
}

type ProviderSpec struct {
	Requirements []ProviderRequirement `yaml:"requirements"`
}

type ProviderRequirement struct {
	Name    string   `yaml:"name"`
	Command []string `yaml:"command"`
}

func (Provider) GetKind() string {
	return "Provider"
}
func (Provider) GetName() string {
	return "provider"
}
