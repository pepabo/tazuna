package runner_test

import (
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
)

func TestConvertManifestPathFromCwd_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	r := runner.NewTazunaRunner(nil, nil, nil)

	original := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "a", Type: v1.ManifestTypeKustomize, Path: "./a"},
				{Name: "b", Type: v1.ManifestTypeKustomize, Path: "./b"},
			},
		},
	}

	// 値コピーは Manifests のスライスヘッダのみで、バッキング配列を共有する。
	// 修正前は、ConvertManifestPathFromCwd が backing 配列の要素を直接書き換えるため
	// original 側まで巻き込まれていた。
	cp := original
	r.ConvertManifestPathFromCwd("/base", &cp)

	if got, want := original.Spec.Manifests[0].Path, "./a"; got != want {
		t.Errorf("original.Manifests[0].Path mutated: got %q, want %q", got, want)
	}
	if got, want := original.Spec.Manifests[1].Path, "./b"; got != want {
		t.Errorf("original.Manifests[1].Path mutated: got %q, want %q", got, want)
	}

	if got, want := cp.Spec.Manifests[0].Path, "/base/a"; got != want {
		t.Errorf("cp.Manifests[0].Path not converted: got %q, want %q", got, want)
	}
	if got, want := cp.Spec.Manifests[1].Path, "/base/b"; got != want {
		t.Errorf("cp.Manifests[1].Path not converted: got %q, want %q", got, want)
	}
}

func TestConvertManifestPathFromCwd_RepeatedCallsAreStableAcrossInputs(t *testing.T) {
	t.Parallel()

	r := runner.NewTazunaRunner(nil, nil, nil)

	original := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "a", Type: v1.ManifestTypeKustomize, Path: "./a"},
			},
		},
	}

	cp1 := original
	r.ConvertManifestPathFromCwd("/base", &cp1)

	cp2 := original
	r.ConvertManifestPathFromCwd("/base", &cp2)

	if got, want := cp2.Spec.Manifests[0].Path, "/base/a"; got != want {
		t.Errorf("second runner call observed mutated input: got %q, want %q (would be /base/base/a before the fix)", got, want)
	}
}
