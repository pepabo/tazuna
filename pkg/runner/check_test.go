package runner_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
)

func TestCheck_OK(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	err := r.Check(context.Background(), &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "kustomize-monitoring", Type: "kustomize", Path: "./monitoring"},
				{Name: "helmfile-example", Type: "helmfile", Path: "./example"},
			},
		},
	}, "testdata/ok/tazuna.yaml")
	assert.NoError(t, err)
}

func TestCheck_NameRequired(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	err := r.Check(context.Background(), &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "", Type: "kustomize", Path: "./monitoring"},
			},
		},
	}, "testdata/ok/tazuna.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestCheck_NameDuplicated(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	err := r.Check(context.Background(), &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "same-name", Type: "kustomize", Path: "./a"},
				{Name: "same-name", Type: "kustomize", Path: "./b"},
			},
		},
	}, "testdata/ok/tazuna.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicated")
}

func TestCheck_NameInvalidChars(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	err := r.Check(context.Background(), &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "invalid name!", Type: "kustomize", Path: "./monitoring"},
			},
		},
	}, "testdata/ok/tazuna.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters")
}

func TestCheck_NameReserved(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	err := r.Check(context.Background(), &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "_metadata", Type: "kustomize", Path: "./monitoring"},
			},
		},
	}, "testdata/ok/tazuna.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

func TestCheck_WithIncludes(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	// includeテストデータの名前にはスペースが含まれているため、バリデーションエラーになる
	err := r.Check(context.Background(), &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Includes: []v1.IncludeFile{
						{Path: "kustomize.yaml"},
						{Path: "genesissecret.yaml"},
					},
				},
			},
		},
	}, "testdata/include/tazuna.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters")
}

func TestCheckAndFix_AssignsNames(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	tazuna := &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Type: "kustomize", Path: "./monitoring"},
			},
		},
	}
	err := r.CheckAndFix(context.Background(), tazuna, "testdata/ok/tazuna.yaml")
	assert.NoError(t, err)
	assert.Equal(t, "kustomize-monitoring", tazuna.Spec.Manifests[0].Name)
}

func TestCheckAndFix_PreservesIncludes(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "include.yaml"), []byte(`apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
    - name: "kustomize-app"
      path: "kustomize"
      type: "kustomize"
`), 0o644)
	assert.NoError(t, err)

	tazuna := &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Includes: []v1.IncludeFile{
						{Path: "include.yaml"},
					},
				},
			},
		},
	}
	err = r.CheckAndFix(context.Background(), tazuna, filepath.Join(dir, "tazuna.yaml"))
	assert.NoError(t, err)

	// CheckAndFix が書き戻し対象の構造体を include 展開で破壊しないことを確認
	assert.Len(t, tazuna.Spec.Manifests, 1)
	assert.Len(t, tazuna.Spec.Manifests[0].Includes, 1)
	// includes を持つ manifest には名前を自動付与しない
	assert.Empty(t, tazuna.Spec.Manifests[0].Name)
}

func TestCheck_WithIncludesValid(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	// include展開が正しく動作し、展開後のmanifestがバリデーションされることを確認
	// 直接構造体を構築してinclude展開をスキップ（展開後の状態をシミュレート）
	err := r.Check(context.Background(), &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "kustomize-deployment", Type: "kustomize", Path: "./a"},
				{Name: "test-secret", Type: "genesissecret", Path: "./b"},
			},
		},
	}, "testdata/ok/tazuna.yaml")
	assert.NoError(t, err)
}
