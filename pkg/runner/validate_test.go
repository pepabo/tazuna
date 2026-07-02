package runner_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
)

// TestApply_StrictNameValidation は --sync / dependsOn 使用時に
// manifest 名バリデーション違反がエラーへ昇格することを確認する。
// (不正名のまま state を書くと誤 prune や誤った依存解決につながるため)
func TestApply_StrictNameValidation(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	duplicated := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "same-name", Type: "kustomize", Path: "./a"},
				{Name: "same-name", Type: "kustomize", Path: "./b"},
			},
		},
	}

	t.Run("sync mode escalates to error", func(t *testing.T) {
		t.Parallel()
		r := runner.NewTazunaRunner(logger, nil, nil,
			runner.WithApplyOptions(runner.ApplyOptions{Sync: true}))
		err := r.Apply(context.Background(), duplicated, "testdata/ok/tazuna.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "manifest name validation failed")
	})

	t.Run("dependsOn escalates to error", func(t *testing.T) {
		t.Parallel()
		withDeps := v1.Tazuna{
			Spec: v1.TazunaSpec{
				Manifests: []v1.Manifest{
					{Name: "Invalid_Name", Type: "kustomize", Path: "./a"},
					{Name: "app", Type: "kustomize", Path: "./b", DependsOn: []string{"Invalid_Name"}},
				},
			},
		}
		r := runner.NewTazunaRunner(logger, nil, nil)
		err := r.Apply(context.Background(), withDeps, "testdata/ok/tazuna.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "manifest name validation failed")
	})
}
