package v1

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProvider_RoundTrip(t *testing.T) {
	t.Parallel()
	src := Provider{
		Spec: ProviderSpec{
			Requirements: []ProviderRequirement{
				{Name: "helm", Command: []string{"helm", "version"}},
				{Name: "kubectl", Command: []string{"kubectl", "version", "--client"}},
			},
		},
	}

	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got Provider
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\nyaml:\n%s", err, string(b))
	}

	if len(got.Spec.Requirements) != 2 {
		t.Fatalf("Requirements len = %d", len(got.Spec.Requirements))
	}
	if got.Spec.Requirements[0].Name != "helm" {
		t.Errorf("Requirements[0].Name = %q", got.Spec.Requirements[0].Name)
	}
	if len(got.Spec.Requirements[0].Command) != 2 || got.Spec.Requirements[0].Command[0] != "helm" {
		t.Errorf("Requirements[0].Command = %+v", got.Spec.Requirements[0].Command)
	}
	if got.Spec.Requirements[1].Name != "kubectl" {
		t.Errorf("Requirements[1].Name = %q", got.Spec.Requirements[1].Name)
	}
	if len(got.Spec.Requirements[1].Command) != 3 {
		t.Errorf("Requirements[1].Command = %+v", got.Spec.Requirements[1].Command)
	}
}

func TestProvider_KindAndName(t *testing.T) {
	t.Parallel()
	p := Provider{}
	if p.GetKind() != "Provider" {
		t.Errorf("GetKind = %q", p.GetKind())
	}
	if p.GetName() != "provider" {
		t.Errorf("GetName = %q", p.GetName())
	}
}
