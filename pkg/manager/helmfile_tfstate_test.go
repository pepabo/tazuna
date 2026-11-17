package manager_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeLookuper は "<source>/<address>" -> 値 の固定マップで tfstate.Lookuper を満たします。
type fakeLookuper map[string]any

func (f fakeLookuper) Lookup(_ context.Context, source, address string) (any, error) {
	v, ok := f[source+"/"+address]
	if !ok {
		return nil, errors.Errorf("tfstate source %q: %q not found", source, address)
	}
	return v, nil
}

func tfstateVarManifest(vars map[string]v1.HelmFileVar) v1.Manifest {
	return v1.Manifest{
		Path:     "testdata/helmfile/helmfile.yaml",
		Type:     v1.ManifestTypeHelmfile,
		Helmfile: &v1.ManifestHelmfile{Vars: vars},
	}
}

func TestHelmfile_ConstructHelmfileVars_TFState(t *testing.T) {
	t.Parallel()
	lookuper := fakeLookuper{
		"network/output.vpc_id":             "vpc-123",
		"network/output.private_subnet_ids": []any{"subnet-a", "subnet-b"},
		"network/output.tags":               map[string]any{"env": "test"},
		"network/output.nothing":            nil,
	}
	m := manager.NewHelmfile(fake.NewFakeClient(), nil).WithTFStateResolver(lookuper)

	t.Run("resolves scalar, list and map values as-is", func(t *testing.T) {
		t.Parallel()
		manifest := tfstateVarManifest(map[string]v1.HelmFileVar{
			"vpcId":   {From: v1.HelmFileVarFromTFState, TFState: &v1.TFStateRef{Source: "network", Address: "output.vpc_id"}},
			"subnets": {From: v1.HelmFileVarFromTFState, TFState: &v1.TFStateRef{Source: "network", Address: "output.private_subnet_ids"}},
			"tags":    {From: v1.HelmFileVarFromTFState, TFState: &v1.TFStateRef{Source: "network", Address: "output.tags"}},
		})
		vars, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		require.NoError(t, err)
		assert.Equal(t, "vpc-123", vars["vpcId"])
		assert.Equal(t, []any{"subnet-a", "subnet-b"}, vars["subnets"])
		assert.Equal(t, map[string]any{"env": "test"}, vars["tags"])
	})

	t.Run("lookup error is wrapped with var name", func(t *testing.T) {
		t.Parallel()
		manifest := tfstateVarManifest(map[string]v1.HelmFileVar{
			"vpcId": {From: v1.HelmFileVarFromTFState, TFState: &v1.TFStateRef{Source: "network", Address: "output.missing"}},
		})
		_, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve helmfile var vpcId from tfstate")
		assert.Contains(t, err.Error(), `"output.missing" not found`)
	})

	t.Run("null value is rejected", func(t *testing.T) {
		t.Parallel()
		manifest := tfstateVarManifest(map[string]v1.HelmFileVar{
			"x": {From: v1.HelmFileVarFromTFState, TFState: &v1.TFStateRef{Source: "network", Address: "output.nothing"}},
		})
		_, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `tfstate value "output.nothing" in source "network" is null`)
	})

	t.Run("missing tfstate field", func(t *testing.T) {
		t.Parallel()
		manifest := tfstateVarManifest(map[string]v1.HelmFileVar{
			"x": {From: v1.HelmFileVarFromTFState},
		})
		_, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tfstate.source / tfstate.address is not set")
	})
}

func TestHelmfile_ConstructHelmfileVars_TFStateWithoutResolver(t *testing.T) {
	t.Parallel()
	m := manager.NewHelmfile(fake.NewFakeClient(), nil)
	manifest := tfstateVarManifest(map[string]v1.HelmFileVar{
		"x": {From: v1.HelmFileVarFromTFState, TFState: &v1.TFStateRef{Source: "network", Address: "output.vpc_id"}},
	})
	_, err := m.ConstructHelmfileVars(context.Background(), &manifest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tfstate resolver is not set")
}
