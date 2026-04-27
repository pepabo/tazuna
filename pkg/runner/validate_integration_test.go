//go:build integration

package runner_test

import (
	"context"
	"log/slog"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// logRecord はログレコードをキャプチャするためのハンドラ
type logRecord struct {
	Level   slog.Level
	Message string
}

type captureHandler struct {
	records *[]logRecord
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	*h.records = append(*h.records, logRecord{Level: r.Level, Message: r.Message})
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func TestApply_WarnManifestNameValidation(t *testing.T) {
	t.Parallel()
	var records []logRecord
	logger := slog.New(&captureHandler{records: &records})
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	// name未設定のmanifestでApplyを実行
	_ = r.Apply(context.Background(), v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "", Type: v1.ManifestTypeKustomize, Path: "testdata/ok/kustomize"},
			},
		},
	}, "testdata/ok/tazuna.yaml")

	// 警告ログが出力されていることを確認
	hasWarn := false
	for _, r := range records {
		if r.Level == slog.LevelWarn {
			hasWarn = true
			assert.Contains(t, r.Message, "manifest name validation failed")
		}
	}
	assert.True(t, hasWarn, "expected warning log for missing manifest name")
}

func TestBuild_WarnManifestNameValidation(t *testing.T) {
	t.Parallel()
	var records []logRecord
	logger := slog.New(&captureHandler{records: &records})
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	// name未設定のmanifestでBuildを実行
	_, _ = r.Build(context.Background(), v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "", Type: v1.ManifestTypeKustomize, Path: "testdata/ok/kustomize"},
			},
		},
	}, "testdata/ok/tazuna.yaml")

	hasWarn := false
	for _, r := range records {
		if r.Level == slog.LevelWarn {
			hasWarn = true
			assert.Contains(t, r.Message, "manifest name validation failed")
		}
	}
	assert.True(t, hasWarn, "expected warning log for missing manifest name")
}

func TestDestroy_WarnManifestNameValidation(t *testing.T) {
	t.Parallel()
	var records []logRecord
	logger := slog.New(&captureHandler{records: &records})
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	// name未設定のmanifestでDestroyを実行
	_ = r.Destroy(context.Background(), v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "", Type: v1.ManifestTypeKustomize, Path: "testdata/ok/kustomize"},
			},
		},
	}, "testdata/ok/tazuna.yaml")

	hasWarn := false
	for _, r := range records {
		if r.Level == slog.LevelWarn {
			hasWarn = true
			assert.Contains(t, r.Message, "manifest name validation failed")
		}
	}
	assert.True(t, hasWarn, "expected warning log for missing manifest name")
}

func TestApply_NoWarnWhenNamesValid(t *testing.T) {
	t.Parallel()
	var records []logRecord
	logger := slog.New(&captureHandler{records: &records})
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	// name設定済みのmanifestでApplyを実行
	_ = r.Apply(context.Background(), v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "kustomize-ok", Type: v1.ManifestTypeKustomize, Path: "testdata/ok/kustomize"},
			},
		},
	}, "testdata/ok/tazuna.yaml")

	// 警告ログが出力されていないことを確認
	for _, r := range records {
		if r.Level == slog.LevelWarn {
			t.Error("unexpected warning log when manifest names are valid")
		}
	}
}
