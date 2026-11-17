package validator_test

import (
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func s3Source(name string) v1.TFStateSource {
	return v1.TFStateSource{Name: name, Backend: v1.TFStateBackend{
		Type: v1.TFStateBackendTypeS3, S3: &v1.TFStateS3Backend{Bucket: "b", Key: "k"},
	}}
}

func TestValidateTFStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sources []v1.TFStateSource
		wantErr string
	}{
		{"empty", nil, ""},
		{"ok", []v1.TFStateSource{s3Source("a"), {Name: "b", Backend: v1.TFStateBackend{Type: "local", Local: &v1.TFStateLocalBackend{Path: "x"}}}}, ""},
		{"missing name", []v1.TFStateSource{{Backend: s3Source("").Backend}}, "tfstates[0]: name is required"},
		{"duplicate", []v1.TFStateSource{s3Source("a"), s3Source("a")}, `tfstates[1]: duplicate name "a"`},
		{"unknown type", []v1.TFStateSource{{Name: "a", Backend: v1.TFStateBackend{Type: "gcs"}}}, `unsupported tfstate backend type "gcs"`},
		{"missing bucket", []v1.TFStateSource{{Name: "a", Backend: v1.TFStateBackend{Type: "s3", S3: &v1.TFStateS3Backend{Key: "k"}}}}, "s3.bucket is required"},
		{"mismatched config", []v1.TFStateSource{{Name: "a", Backend: v1.TFStateBackend{
			Type: "s3", S3: &v1.TFStateS3Backend{Bucket: "b", Key: "k"}, Local: &v1.TFStateLocalBackend{Path: "x"},
		}}}, `backend.type is "s3" but config for [local] is also set`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validator.ValidateTFStates(tt.sources)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateHelmFileVar_TFState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ref     *v1.TFStateRef
		wantErr string
	}{
		{"ok", &v1.TFStateRef{Source: "a", Address: "output.x"}, ""},
		{"missing field", nil, "tfstate field is required"},
		{"missing source", &v1.TFStateRef{Address: "output.x"}, "tfstate.source is required"},
		{"missing address", &v1.TFStateRef{Source: "a"}, "tfstate.address is required"},
		{"invalid address", &v1.TFStateRef{Source: "a", Address: "aws_vpc.main"}, "invalid tfstate address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validator.ValidateHelmFileVar(&v1.HelmFileVar{From: v1.HelmFileVarFromTFState, TFState: tt.ref}, "v")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateTazunaSpec_TFStateReferences(t *testing.T) {
	t.Parallel()

	helmfileManifest := func(source string) v1.Manifest {
		return v1.Manifest{Name: "app", Type: v1.ManifestTypeHelmfile, Path: "./app", Helmfile: &v1.ManifestHelmfile{
			Vars: map[string]v1.HelmFileVar{
				"vpcId": {From: v1.HelmFileVarFromTFState, TFState: &v1.TFStateRef{Source: source, Address: "output.vpc_id"}},
			},
		}}
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		spec := v1.TazunaSpec{
			TFStates:  []v1.TFStateSource{s3Source("network")},
			Manifests: []v1.Manifest{helmfileManifest("network")},
			Providers: []v1.ProviderConfig{{Name: "tf", Type: v1.ProviderTypeTFState, TFState: &v1.TFStateProviderConfig{Source: "network"}}},
		}
		require.NoError(t, validator.ValidateTazunaSpec(&spec, ""))
	})

	t.Run("helmfile var refers to undeclared source", func(t *testing.T) {
		t.Parallel()
		spec := v1.TazunaSpec{
			TFStates:  []v1.TFStateSource{s3Source("network")},
			Manifests: []v1.Manifest{helmfileManifest("storage")},
		}
		err := validator.ValidateTazunaSpec(&spec, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `helmfile var 'vpcId' refers to tfstate source "storage" which is not declared in spec.tfstates`)
	})

	t.Run("provider refers to undeclared source", func(t *testing.T) {
		t.Parallel()
		spec := v1.TazunaSpec{
			Providers: []v1.ProviderConfig{{Name: "tf", Type: v1.ProviderTypeTFState, TFState: &v1.TFStateProviderConfig{Source: "network"}}},
		}
		err := validator.ValidateTazunaSpec(&spec, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `providers[0] ("tf"): tfstate source "network" is not declared in spec.tfstates`)
	})

	t.Run("provider without source", func(t *testing.T) {
		t.Parallel()
		spec := v1.TazunaSpec{
			Providers: []v1.ProviderConfig{{Name: "tf", Type: v1.ProviderTypeTFState, TFState: &v1.TFStateProviderConfig{}}},
		}
		err := validator.ValidateTazunaSpec(&spec, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tfstate.source is required")
	})

	t.Run("provider without config", func(t *testing.T) {
		t.Parallel()
		spec := v1.TazunaSpec{
			Providers: []v1.ProviderConfig{{Name: "tf", Type: v1.ProviderTypeTFState}},
		}
		err := validator.ValidateTazunaSpec(&spec, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tfstate config is required for tfstate type")
	})
}
