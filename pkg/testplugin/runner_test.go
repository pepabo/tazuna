package testplugin_test

import (
	"context"
	"fmt"
	"testing"
	"time"

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
		IntervalSeconds:            1,
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
		IntervalSeconds:            1,
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

func TestStart_ContextCancel(t *testing.T) {
	t.Parallel()
	// ctxのキャンセルでループが停止することを確認する (Ctrl+C相当)
	spec := &v1.TestPluginSpec{}
	f := func(ctx context.Context, logger *slog.Logger, spec *v1.TestPluginSpec) error {
		return fmt.Errorf("never succeeds")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := testplugin.Start(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), spec, f)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	// デフォルトinterval (2s) の待機中でもキャンセルが即座に効くこと
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestStart_SuccessAfterFailures(t *testing.T) {
	t.Parallel()
	// 失敗が続いた後の連続成功で通過することを確認する
	// (履歴を直近max件に打ち切っても判定が壊れないことの確認)
	spec := &v1.TestPluginSpec{
		MinConsecutiveSuccessCount: 2,
		MinConsecutiveFailureCount: 10,
		IntervalSeconds:            1,
	}
	count := 0
	f := func(ctx context.Context, logger *slog.Logger, spec *v1.TestPluginSpec) error {
		count++
		if count <= 3 {
			return fmt.Errorf("not yet")
		}
		return nil
	}

	err := testplugin.Start(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), spec, f)
	assert.NoError(t, err)
	assert.Equal(t, 5, count)
}
