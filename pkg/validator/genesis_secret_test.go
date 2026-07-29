package validator_test

import (
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/stretchr/testify/assert"
)

func TestValidateGenesisSecretSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    *v1.GenesisSecretSpec
		wantErr bool
	}{
		{
			name: "valid op uri with default provider",
			spec: &v1.GenesisSecretSpec{
				Secrets: []v1.GenesisSecretGenerate{
					{URI: "op://example.1password.com/example-vault/cloud-credentials"},
				},
			},
		},
		{
			name: "malformed op uri with default provider",
			spec: &v1.GenesisSecretSpec{
				Secrets: []v1.GenesisSecretGenerate{
					{URI: "op://example-vault/cloud-credentials"},
				},
			},
			wantErr: true,
		},
		{
			name: "non-op provider skips uri validation",
			spec: &v1.GenesisSecretSpec{
				Provider: "my-envfile",
				Secrets: []v1.GenesisSecretGenerate{
					{URI: ""},
				},
			},
		},
		{
			name:    "nil spec",
			spec:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validator.ValidateGenesisSecretSpec(tt.spec)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateManifestGenesisSecret(t *testing.T) {
	t.Parallel()

	writeFile := func(t *testing.T, content string) (dir, name string) {
		t.Helper()
		dir = t.TempDir()
		name = "genesissecret.yaml"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir, name
	}

	t.Run("valid uri passes", func(t *testing.T) {
		t.Parallel()
		dir, name := writeFile(t, `
spec:
  secrets:
    - uri: op://example.1password.com/example-vault/item
      items:
        password:
          mapTo: PASSWORD
`)
		m := &v1.Manifest{Path: name, Type: v1.ManifestTypeGenesisSecret}
		assert.NoError(t, validator.ValidateManifestGenesisSecret(m, dir))
	})

	t.Run("malformed uri fails", func(t *testing.T) {
		t.Parallel()
		dir, name := writeFile(t, `
spec:
  secrets:
    - uri: op://example-vault/item
      items:
        password:
          mapTo: PASSWORD
`)
		m := &v1.Manifest{Path: name, Type: v1.ManifestTypeGenesisSecret}
		err := validator.ValidateManifestGenesisSecret(m, dir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "op://<host>/<vault>/<item>")
	})

	t.Run("missing file is not an error here", func(t *testing.T) {
		t.Parallel()
		m := &v1.Manifest{Path: "no-such-file.yaml", Type: v1.ManifestTypeGenesisSecret}
		assert.NoError(t, validator.ValidateManifestGenesisSecret(m, t.TempDir()))
	})
}
