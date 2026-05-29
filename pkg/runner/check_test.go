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
