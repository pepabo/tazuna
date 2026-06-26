package oras

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mockPuller は Puller interface のモック実装。
type mockPuller struct {
	result PullResult
	err    error

	calls    int
	lastCtx  context.Context
	lastSpec v1.ManifestORAS
	lastOpts PullOptions
}

func (m *mockPuller) Pull(ctx context.Context, _ *slog.Logger, spec v1.ManifestORAS, opts PullOptions) (PullResult, error) {
	m.calls++
	m.lastCtx = ctx
	m.lastSpec = spec
	m.lastOpts = opts
	if m.err != nil {
		return PullResult{}, m.err
	}
	return m.result, nil
}

// mockDelegate は DelegateManager interface のモック実装。
type mockDelegate struct {
	applyErr   error
	destroyErr error
	buildOut   string
	buildErr   error

	applyCalls   int
	destroyCalls int
	buildCalls   int
	lastManifest v1.Manifest
}

func (m *mockDelegate) Apply(_ context.Context, _ *slog.Logger, mf v1.Manifest) ([]client.Object, error) {
	m.applyCalls++
	m.lastManifest = mf
	return nil, m.applyErr
}

func (m *mockDelegate) Destroy(_ context.Context, _ *slog.Logger, mf v1.Manifest) error {
	m.destroyCalls++
	m.lastManifest = mf
	return m.destroyErr
}

func (m *mockDelegate) Build(_ context.Context, _ *slog.Logger, mf v1.Manifest) (string, error) {
	m.buildCalls++
	m.lastManifest = mf
	return m.buildOut, m.buildErr
}

func newTestORAS(t *testing.T, localPath string) (*ORAS, *mockPuller, *mockDelegate, *mockDelegate) {
	t.Helper()
	puller := &mockPuller{result: PullResult{LocalPath: localPath, Digest: "sha256:deadbeef"}}
	hm := &mockDelegate{}
	km := &mockDelegate{}
	return New(puller, hm, km), puller, hm, km
}

func TestORAS_Apply_DelegatesToHelmfile(t *testing.T) {
	tmp := t.TempDir()
	helmSpec := &v1.ManifestHelmfile{IncludeCRDs: true, DefaultNamespace: "kube-system"}
	o, puller, hm, km := newTestORAS(t, tmp)

	m := v1.Manifest{
		Name: "test",
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/foo:v1",
			Delegate: v1.ORASDelegate{
				Type:     v1.ORASDelegateTypeHelmfile,
				Helmfile: helmSpec,
			},
		},
	}

	if _, err := o.Apply(context.Background(), slog.Default(), m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if puller.calls != 1 {
		t.Fatalf("puller.calls = %d, want 1", puller.calls)
	}
	if hm.applyCalls != 1 {
		t.Fatalf("helmfile.applyCalls = %d, want 1", hm.applyCalls)
	}
	if km.applyCalls != 0 {
		t.Fatalf("kustomize.applyCalls = %d, want 0", km.applyCalls)
	}
	got := hm.lastManifest
	if got.Type != v1.ManifestTypeHelmfile {
		t.Errorf("delegated.Type = %q, want helmfile", got.Type)
	}
	if got.Path != tmp {
		t.Errorf("delegated.Path = %q, want %q", got.Path, tmp)
	}
	if got.Helmfile != helmSpec {
		t.Errorf("delegated.Helmfile pointer not propagated")
	}
	if got.Name != "test" {
		t.Errorf("delegated.Name = %q, want test", got.Name)
	}
}

func TestORAS_Apply_DelegatesToKustomizeWithTargetSubpath(t *testing.T) {
	tmp := t.TempDir()
	kustSpec := &v1.ManifestKustomize{DefaultNamespace: "default"}
	o, _, _, km := newTestORAS(t, tmp)

	m := v1.Manifest{
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/bar@sha256:abc",
			Target:    "overlays/dev",
			Delegate: v1.ORASDelegate{
				Type:      v1.ORASDelegateTypeKustomize,
				Kustomize: kustSpec,
			},
		},
	}

	if _, err := o.Apply(context.Background(), nil, m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if km.applyCalls != 1 {
		t.Fatalf("kustomize.applyCalls = %d, want 1", km.applyCalls)
	}
	want := filepath.Join(tmp, "overlays/dev")
	if km.lastManifest.Path != want {
		t.Errorf("delegated.Path = %q, want %q", km.lastManifest.Path, want)
	}
	if km.lastManifest.Type != v1.ManifestTypeKustomize {
		t.Errorf("delegated.Type = %q, want kustomize", km.lastManifest.Type)
	}
	if km.lastManifest.Kustomize != kustSpec {
		t.Errorf("delegated.Kustomize pointer not propagated")
	}
}

func TestORAS_Apply_TargetEscapeRejected(t *testing.T) {
	tmp := t.TempDir()
	o, _, _, _ := newTestORAS(t, tmp)

	m := v1.Manifest{
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/foo:v1",
			Target:    "../etc",
			Delegate: v1.ORASDelegate{
				Type:      v1.ORASDelegateTypeKustomize,
				Kustomize: &v1.ManifestKustomize{},
			},
		},
	}
	_, err := o.Apply(context.Background(), nil, m)
	if err == nil || !strings.Contains(err.Error(), "invalid target path") {
		t.Fatalf("Apply error = %v, want invalid target path", err)
	}
}

func TestORAS_Apply_UnsupportedDelegateType(t *testing.T) {
	tmp := t.TempDir()
	o, _, _, _ := newTestORAS(t, tmp)

	cases := []v1.ORASDelegateType{"", "genesissecret", "parallel"}
	for _, dt := range cases {
		m := v1.Manifest{
			Type: v1.ManifestTypeORAS,
			ORAS: &v1.ManifestORAS{
				Reference: "ghcr.io/example/foo:v1",
				Delegate:  v1.ORASDelegate{Type: dt},
			},
		}
		_, err := o.Apply(context.Background(), nil, m)
		if err == nil || !strings.Contains(err.Error(), "unsupported delegate type") {
			t.Errorf("delegate %q: error = %v, want unsupported delegate type", dt, err)
		}
	}
}

func TestORAS_Apply_NilORASSpec(t *testing.T) {
	o, _, _, _ := newTestORAS(t, t.TempDir())
	_, err := o.Apply(context.Background(), nil, v1.Manifest{Name: "x", Type: v1.ManifestTypeORAS})
	if err == nil || !strings.Contains(err.Error(), "no oras spec") {
		t.Fatalf("error = %v, want no oras spec", err)
	}
}

func TestORAS_Apply_PullerErrorPropagates(t *testing.T) {
	tmp := t.TempDir()
	puller := &mockPuller{err: errors.New("boom")}
	o := New(puller, &mockDelegate{}, &mockDelegate{})

	m := v1.Manifest{
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/foo:v1",
			Delegate:  v1.ORASDelegate{Type: v1.ORASDelegateTypeHelmfile, Helmfile: &v1.ManifestHelmfile{}},
		},
	}
	_, err := o.Apply(context.Background(), nil, m)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pull") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want wrapped pull error", err)
	}
	_ = tmp
}

func TestORAS_Apply_DelegateErrorPropagates(t *testing.T) {
	tmp := t.TempDir()
	hm := &mockDelegate{applyErr: errors.New("apply failed")}
	o := New(&mockPuller{result: PullResult{LocalPath: tmp}}, hm, &mockDelegate{})

	m := v1.Manifest{
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/foo:v1",
			Delegate:  v1.ORASDelegate{Type: v1.ORASDelegateTypeHelmfile, Helmfile: &v1.ManifestHelmfile{}},
		},
	}
	_, err := o.Apply(context.Background(), nil, m)
	if err == nil || !strings.Contains(err.Error(), "apply failed") {
		t.Fatalf("error = %v, want apply failed", err)
	}
}

func TestORAS_Build_DelegatesAndReturnsString(t *testing.T) {
	tmp := t.TempDir()
	hm := &mockDelegate{buildOut: "rendered yaml"}
	o := New(&mockPuller{result: PullResult{LocalPath: tmp}}, hm, &mockDelegate{})

	m := v1.Manifest{
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/foo:v1",
			Delegate:  v1.ORASDelegate{Type: v1.ORASDelegateTypeHelmfile, Helmfile: &v1.ManifestHelmfile{}},
		},
	}
	got, err := o.Build(context.Background(), nil, m)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got != "rendered yaml" {
		t.Errorf("Build returned %q, want %q", got, "rendered yaml")
	}
	if hm.buildCalls != 1 {
		t.Errorf("buildCalls = %d, want 1", hm.buildCalls)
	}
}

func TestORAS_Destroy_Delegates(t *testing.T) {
	tmp := t.TempDir()
	km := &mockDelegate{}
	o := New(&mockPuller{result: PullResult{LocalPath: tmp}}, &mockDelegate{}, km)

	m := v1.Manifest{
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/bar:v2",
			Delegate:  v1.ORASDelegate{Type: v1.ORASDelegateTypeKustomize, Kustomize: &v1.ManifestKustomize{}},
		},
	}
	if err := o.Destroy(context.Background(), nil, m); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if km.destroyCalls != 1 {
		t.Errorf("destroyCalls = %d, want 1", km.destroyCalls)
	}
	if km.lastManifest.Path != tmp {
		t.Errorf("delegated.Path = %q, want %q", km.lastManifest.Path, tmp)
	}
}

func TestORAS_NewWithOptions_PassesPullOptions(t *testing.T) {
	tmp := t.TempDir()
	puller := &mockPuller{result: PullResult{LocalPath: tmp}}
	opts := PullOptions{NoCache: true, Offline: false, CacheDir: "/tmp/cache"}
	o := NewWithOptions(puller, &mockDelegate{}, &mockDelegate{}, opts)

	m := v1.Manifest{
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/foo:v1",
			Delegate:  v1.ORASDelegate{Type: v1.ORASDelegateTypeHelmfile, Helmfile: &v1.ManifestHelmfile{}},
		},
	}
	if _, err := o.Apply(context.Background(), nil, m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if puller.lastOpts != opts {
		t.Errorf("puller.lastOpts = %+v, want %+v", puller.lastOpts, opts)
	}
}

func TestORAS_NilDelegateConfigured(t *testing.T) {
	tmp := t.TempDir()
	// helmfile delegate is nil
	o := New(&mockPuller{result: PullResult{LocalPath: tmp}}, nil, &mockDelegate{})

	m := v1.Manifest{
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/foo:v1",
			Delegate:  v1.ORASDelegate{Type: v1.ORASDelegateTypeHelmfile, Helmfile: &v1.ManifestHelmfile{}},
		},
	}
	_, err := o.Apply(context.Background(), nil, m)
	if err == nil || !strings.Contains(err.Error(), "helmfile delegate is not configured") {
		t.Fatalf("error = %v, want helmfile delegate not configured", err)
	}
}

func TestORAS_TargetWithLeadingSlashStripped(t *testing.T) {
	tmp := t.TempDir()
	km := &mockDelegate{}
	o := New(&mockPuller{result: PullResult{LocalPath: tmp}}, &mockDelegate{}, km)

	m := v1.Manifest{
		Type: v1.ManifestTypeORAS,
		ORAS: &v1.ManifestORAS{
			Reference: "ghcr.io/example/bar:v1",
			Target:    "/charts/app",
			Delegate:  v1.ORASDelegate{Type: v1.ORASDelegateTypeKustomize, Kustomize: &v1.ManifestKustomize{}},
		},
	}
	if _, err := o.Apply(context.Background(), nil, m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := filepath.Join(tmp, "charts/app")
	if km.lastManifest.Path != want {
		t.Errorf("delegated.Path = %q, want %q", km.lastManifest.Path, want)
	}
}
