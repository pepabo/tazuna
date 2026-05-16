package v1

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
