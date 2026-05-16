package v1

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestTazunaHint_RoundTrip(t *testing.T) {
	t.Parallel()
	src := TazunaHint{
		APIVersion: "tazuna.pepabo.com/v1",
		Kind:       "TazunaHint",
		Vars: map[string]HintVar{
			"domain": {
				Type:        HintVarTypeString,
				Required:    true,
				Description: "hostname",
				Format:      HintFormatHostname,
			},
			"replicas": {
				Type:    HintVarTypeString,
				Default: "3",
			},
			"endpoint": {
				Type:         HintVarTypeString,
				Format:       HintFormatURL,
				RequiredWith: []string{"token"},
			},
			"token": {
				Type:            HintVarTypeString,
				RequiredWithout: []string{"endpoint"},
			},
		},
		Rules: []HintRule{
			{
				Type:    HintRuleTypeOneofRequired,
				Vars:    []string{"a", "b"},
				Message: "either a or b is required",
			},
		},
	}

	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got TazunaHint
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\nyaml:\n%s", err, string(b))
	}

	if got.APIVersion != "tazuna.pepabo.com/v1" || got.Kind != "TazunaHint" {
		t.Errorf("APIVersion=%q Kind=%q", got.APIVersion, got.Kind)
	}
	if len(got.Vars) != 4 {
		t.Fatalf("Vars len = %d", len(got.Vars))
	}
	if got.Vars["domain"].Type != HintVarTypeString || got.Vars["domain"].Format != HintFormatHostname {
		t.Errorf("domain var = %+v", got.Vars["domain"])
	}
	if !got.Vars["domain"].Required {
		t.Error("domain.Required = false, want true")
	}
	if got.Vars["endpoint"].RequiredWith == nil || got.Vars["endpoint"].RequiredWith[0] != "token" {
		t.Errorf("endpoint.RequiredWith = %+v", got.Vars["endpoint"].RequiredWith)
	}
	if got.Vars["token"].RequiredWithout == nil || got.Vars["token"].RequiredWithout[0] != "endpoint" {
		t.Errorf("token.RequiredWithout = %+v", got.Vars["token"].RequiredWithout)
	}
	if len(got.Rules) != 1 || got.Rules[0].Type != HintRuleTypeOneofRequired {
		t.Errorf("Rules = %+v", got.Rules)
	}
	if got.Rules[0].Message != "either a or b is required" {
		t.Errorf("Rules[0].Message = %q", got.Rules[0].Message)
	}
}

func TestHintVar_RoundTrip_AllTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		t    HintVarType
	}{
		{"string", HintVarTypeString},
		{"slice", HintVarTypeSlice},
		{"map", HintVarTypeMap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := HintVar{Type: tt.t}
			b, err := yaml.Marshal(src)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			var got HintVar
			if err := yaml.Unmarshal(b, &got); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			if got.Type != tt.t {
				t.Errorf("Type = %q, want %q", got.Type, tt.t)
			}
		})
	}
}

func TestHintVar_RoundTrip_AllFormats(t *testing.T) {
	t.Parallel()
	formats := []HintFormat{
		HintFormatHostname,
		HintFormatURL,
		HintFormatEmail,
		HintFormatIP,
		HintFormatCIDR,
		HintFormatUUID,
		HintFormatSemver,
		HintFormatDatetime,
	}
	for _, f := range formats {
		t.Run(string(f), func(t *testing.T) {
			t.Parallel()
			src := HintVar{Type: HintVarTypeString, Format: f}
			b, err := yaml.Marshal(src)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			var got HintVar
			if err := yaml.Unmarshal(b, &got); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			if got.Format != f {
				t.Errorf("Format = %q, want %q", got.Format, f)
			}
		})
	}
}

func TestTazunaHint_OmitEmptyRules(t *testing.T) {
	t.Parallel()
	src := TazunaHint{
		APIVersion: "v1",
		Kind:       "TazunaHint",
		Vars:       map[string]HintVar{},
	}
	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	if containsAny(string(b), "rules:") {
		t.Errorf("rules should be omitted, got:\n%s", string(b))
	}
}
