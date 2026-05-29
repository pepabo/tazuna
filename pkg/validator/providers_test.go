package validator

import (
	"strings"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
)

func TestValidateProviders_Empty(t *testing.T) {
	t.Parallel()
	if err := ValidateProviders(nil); err != nil {
		t.Errorf("nil providers should pass, got %v", err)
	}
	if err := ValidateProviders([]v1.ProviderConfig{}); err != nil {
		t.Errorf("empty providers should pass, got %v", err)
	}
}

func TestValidateProviders_HappyEnvFile(t *testing.T) {
	t.Parallel()
	err := ValidateProviders([]v1.ProviderConfig{
		{
			Name:    "my-env",
			Type:    v1.ProviderTypeEnvFile,
			EnvFile: &v1.EnvFileProviderConfig{Path: "secrets.env"},
		},
	})
	if err != nil {
		t.Errorf("expected pass, got %v", err)
	}
}

func TestValidateProviders_DuplicateName(t *testing.T) {
	t.Parallel()
	err := ValidateProviders([]v1.ProviderConfig{
		{Name: "p", Type: v1.ProviderTypeEnvFile, EnvFile: &v1.EnvFileProviderConfig{Path: "a.env"}},
		{Name: "p", Type: v1.ProviderTypeEnvFile, EnvFile: &v1.EnvFileProviderConfig{Path: "b.env"}},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate error, got %v", err)
	}
}

func TestValidateProviders_ReservedName(t *testing.T) {
	t.Parallel()
	err := ValidateProviders([]v1.ProviderConfig{
		{
			Name:    v1.DefaultOnePasswordProviderName,
			Type:    v1.ProviderTypeEnvFile,
			EnvFile: &v1.EnvFileProviderConfig{Path: "a.env"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("expected reserved error, got %v", err)
	}
}

func TestValidateProviders_EmptyName(t *testing.T) {
	t.Parallel()
	err := ValidateProviders([]v1.ProviderConfig{
		{Type: v1.ProviderTypeEnvFile, EnvFile: &v1.EnvFileProviderConfig{Path: "a.env"}},
	})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected empty name error, got %v", err)
	}
}

func TestValidateProviders_EmptyType(t *testing.T) {
	t.Parallel()
	err := ValidateProviders([]v1.ProviderConfig{{Name: "x"}})
	if err == nil || !strings.Contains(err.Error(), "type is required") {
		t.Errorf("expected empty type error, got %v", err)
	}
}

func TestValidateProviders_UnsupportedType(t *testing.T) {
	t.Parallel()
	err := ValidateProviders([]v1.ProviderConfig{{Name: "x", Type: "bogus"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported provider type") {
		t.Errorf("expected unsupported error, got %v", err)
	}
}

func TestValidateProviders_EnvFileMissingPath(t *testing.T) {
	t.Parallel()
	err := ValidateProviders([]v1.ProviderConfig{
		{Name: "x", Type: v1.ProviderTypeEnvFile, EnvFile: &v1.EnvFileProviderConfig{}},
	})
	if err == nil || !strings.Contains(err.Error(), "envfile.path is required") {
		t.Errorf("expected path required error, got %v", err)
	}
}

func TestValidateProviders_EnvFileMissingConfig(t *testing.T) {
	t.Parallel()
	err := ValidateProviders([]v1.ProviderConfig{
		{Name: "x", Type: v1.ProviderTypeEnvFile},
	})
	if err == nil || !strings.Contains(err.Error(), "envfile config is required") {
		t.Errorf("expected envfile config required error, got %v", err)
	}
}

func TestValidateProviders_OnePasswordExplicitRejected(t *testing.T) {
	t.Parallel()
	err := ValidateProviders([]v1.ProviderConfig{
		{Name: "extra-op", Type: v1.ProviderTypeOnePassword},
	})
	if err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Errorf("expected onepassword explicit unsupported error, got %v", err)
	}
}
