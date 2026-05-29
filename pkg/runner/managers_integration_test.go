//go:build integration

package runner

import (
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	orasmanager "github.com/pepabo/tazuna/pkg/manager/oras"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSetupManagers_RegistersORAS(t *testing.T) {
	t.Parallel()
	k8sClient := fake.NewClientBuilder().Build()
	managers, err := setupManagers(k8sClient, nil, orasmanager.PullOptions{}, nil, "")
	if err != nil {
		t.Fatalf("setupManagers returned error: %v", err)
	}

	mgr, ok := managers[string(v1.ManifestTypeORAS)]
	if !ok {
		t.Fatalf("ORAS manager is not registered")
	}
	if _, ok := mgr.(*orasmanager.ORAS); !ok {
		t.Fatalf("registered ORAS manager has unexpected type %T", mgr)
	}
}

func TestSetupManagers_RegistersAllExpectedTypes(t *testing.T) {
	t.Parallel()
	k8sClient := fake.NewClientBuilder().Build()
	managers, err := setupManagers(k8sClient, nil, orasmanager.PullOptions{}, nil, "")
	if err != nil {
		t.Fatalf("setupManagers returned error: %v", err)
	}

	expected := []v1.ManifestType{
		v1.ManifestTypeGenesisSecret,
		v1.ManifestTypeKustomize,
		v1.ManifestTypeHelmfile,
		v1.ManifestTypeORAS,
	}
	for _, mt := range expected {
		if _, ok := managers[string(mt)]; !ok {
			t.Errorf("manifest type %q is not registered", mt)
		}
	}
}
