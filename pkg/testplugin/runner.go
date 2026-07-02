package testplugin

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// defaultIntervalSeconds はIntervalSeconds未設定時のテスト関数実行間隔。
// テスト関数は毎回k8s APIへリクエストするため、無間隔での再実行
// (busy loop) を防ぐデフォルト値を設ける。
const defaultIntervalSeconds = 2

// Startは各テストプラグインに依存しない共通的な挙動を実装する高階関数です
// specの設定に基づき、引数に渡されたテスト関数を実行ループします
//
// TimeoutSeconds が 0 の場合はタイムアウトせず、成功条件を満たすか
// ctx がキャンセルされるまで待ち続けます。
func Start(
	ctx context.Context,
	logger *slog.Logger,
	spec *v1.TestPluginSpec,
	f func(ctx context.Context, logger *slog.Logger, spec *v1.TestPluginSpec) error,
) error {
	if spec.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(spec.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	interval := time.Duration(spec.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = defaultIntervalSeconds * time.Second
	}

	// MinConcectiveSuccessCountは常に1にする
	if spec.MinConsecutiveSuccessCount == 0 {
		spec.MinConsecutiveSuccessCount = 1
	}

	// 連続成功/失敗の判定に必要なのは直近 max(成功回数, 失敗回数) 件のみなので、
	// 履歴の保持数をその上界で打ち切る
	maxHistory := max(spec.MinConsecutiveSuccessCount, spec.MinConsecutiveFailureCount)
	testFnHistory := make([]error, 0, maxHistory)

	// 初回は即時実行し、以降はintervalごとに実行する
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("aborting test: %w", ctx.Err())
		case <-timer.C:
		}

		err := f(ctx, logger, spec)

		testFnHistory = append(testFnHistory, err)
		if len(testFnHistory) > maxHistory {
			copy(testFnHistory, testFnHistory[len(testFnHistory)-maxHistory:])
			testFnHistory = testFnHistory[:maxHistory]
		}

		// 設定されているときだけチェックする
		// 常に0ではないのでチェックしない
		if minConcectiveSuccessReached(testFnHistory, spec.MinConsecutiveSuccessCount) {
			return nil
		}
		if spec.MinConsecutiveFailureCount != 0 && minConcectiveFailureReached(testFnHistory, spec.MinConsecutiveFailureCount) {
			return fmt.Errorf("aborting test after %d consecutive failures (last error: \"%s\")", spec.MinConsecutiveFailureCount, testFnHistory[len(testFnHistory)-1].Error())
		}

		timer.Reset(interval)
	}
}

// 直近でN回連続で失敗したかどうか、を履歴から検索する
func minConcectiveFailureReached(testFnHistory []error, count int) bool {
	// 履歴が十分に溜まっていなかったらなにもしない
	if len(testFnHistory) < count {
		return false
	}

	// 開始するオフセットの計算
	// 例えば []bool{error, nil, nil} で、count=2なら、
	//                     ↑から始める
	// 例えば []bool{error, nil, nil, error, error} で、count=3なら、
	//                            ↑から始める
	offset := len(testFnHistory) - count

	for _, err := range testFnHistory[offset:] {
		if err == nil {
			// 一度でも成功しているのでfalse
			return false
		}
	}
	return true
}

// 直近でN回連続で成功したかどうか、を履歴から検索する
func minConcectiveSuccessReached(testFnHistory []error, count int) bool {
	// 履歴が十分に溜まっていなかったらなにもしない
	if len(testFnHistory) < count {
		return false
	}

	// 開始するオフセットの計算
	// 例えば []bool{true, false, false} で、count=2なら、
	//                     ↑から始める
	offset := len(testFnHistory) - count

	for _, err := range testFnHistory[offset:] {
		if err != nil {
			// 一度でも失敗しているのでfalse
			return false
		}
	}

	return true
}
