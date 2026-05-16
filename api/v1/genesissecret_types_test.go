package v1

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestGenesisSecret_RoundTrip(t *testing.T) {
	t.Parallel()
	src := GenesisSecret{
		Spec: GenesisSecretSpec{
			Provider: "1password",
			Secrets: []GenesisSecretGenerate{
				{
					PreferLabel: true,
					URI:         "op://my-vault/my-item",
					Items: map[string]GenesisSecretGenerateItem{
						"username": {MapTo: "user"},
						"password": {MapTo: "pass"},
					},
				},
			},
			Outputs: []GenesisSecretOutput{
				{
					KubernetesSecret: &GenesisSecretOutputKubernetesSecret{
						Context:     "kind-tazuna",
						Namespace:   "default",
						Name:        "app-secret",
						Labels:      map[string]string{"app": "myapp"},
						Annotations: map[string]string{"managed-by": "tazuna"},
						Type:        "Opaque",
					},
				},
			},
		},
	}

	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got GenesisSecret
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\nyaml:\n%s", err, string(b))
	}

	if got.Spec.Provider != "1password" {
		t.Errorf("Provider = %q", got.Spec.Provider)
	}
	if len(got.Spec.Secrets) != 1 {
		t.Fatalf("Secrets len = %d", len(got.Spec.Secrets))
	}
	s := got.Spec.Secrets[0]
	if !s.PreferLabel {
		t.Errorf("PreferLabel = false, want true")
	}
	if s.URI != "op://my-vault/my-item" {
		t.Errorf("URI = %q", s.URI)
	}
	if len(s.Items) != 2 || s.Items["username"].MapTo != "user" || s.Items["password"].MapTo != "pass" {
		t.Errorf("Items = %+v", s.Items)
	}

	if len(got.Spec.Outputs) != 1 {
		t.Fatalf("Outputs len = %d", len(got.Spec.Outputs))
	}
	o := got.Spec.Outputs[0].KubernetesSecret
	if o == nil {
		t.Fatal("KubernetesSecret nil")
	}
	if o.Name != "app-secret" || o.Namespace != "default" || o.Type != "Opaque" {
		t.Errorf("KubernetesSecret = %+v", o)
	}
	if o.Labels["app"] != "myapp" || o.Annotations["managed-by"] != "tazuna" {
		t.Errorf("labels/annotations lost: %+v / %+v", o.Labels, o.Annotations)
	}
}

func TestGenesisSecretOutput_StdoutOnly(t *testing.T) {
	t.Parallel()
	src := GenesisSecretOutput{
		Stdout: &GenesisSecretOutputStdout{},
	}
	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got GenesisSecretOutput
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if got.Stdout == nil {
		t.Error("Stdout = nil, want non-nil")
	}
	if got.KubernetesSecret != nil {
		t.Errorf("KubernetesSecret = %+v, want nil (omitted)", got.KubernetesSecret)
	}
}

