package testplugin_test

import (
	"context"
	"fmt"
	"testing"

	"io"
	"log/slog"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/testplugin"
	"github.com/stretchr/testify/assert"
)

func TestStart_Empty(t *testing.T) {
	t.Parallel()
	// 全て空のときも、MinConsecutiveSuccessCountが1として動いてほしい
	spec := &v1.TestPluginSpec{}
	f := func(ctx context.Context, logger *slog.Logger, spec *v1.TestPluginSpec) error {
		return nil
	}

	err := testplugin.Start(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), spec, f)
	assert.NoError(t, err)
}

func TestStart_Timeout(t *testing.T) {
	t.Parallel()
	spec := &v1.TestPluginSpec{
		TimeoutSeconds: 2,
	}
	// 常に終わらないテストを作って、timeoutでテストが終了されるかを見る
	f := func(ctx context.Context, logger *slog.Logger, spec *v1.TestPluginSpec) error {
		return fmt.Errorf("test")
	}

	err := testplugin.Start(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), spec, f)
	assert.Error(t, err)
}

func TestStart_MinConsectiveFailure(t *testing.T) {
	t.Parallel()
	spec := &v1.TestPluginSpec{
		MinConsecutiveFailureCount: 3,
	}
	count := 0
	f := func(ctx context.Context, logger *slog.Logger, spec *v1.TestPluginSpec) error {
		count++
		return fmt.Errorf("err")
	}

	err := testplugin.Start(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), spec, f)
	assert.Error(t, err)
	assert.Equal(t, count, spec.MinConsecutiveFailureCount)
}

func TestStart_MinConsectiveSuccess(t *testing.T) {
	t.Parallel()
	spec := &v1.TestPluginSpec{
		MinConsecutiveSuccessCount: 3,
	}
	count := 0
	f := func(ctx context.Context, logger *slog.Logger, spec *v1.TestPluginSpec) error {
		count++
		return nil
	}

	err := testplugin.Start(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), spec, f)
	assert.NoError(t, err)
	assert.Equal(t, count, spec.MinConsecutiveSuccessCount)
}
