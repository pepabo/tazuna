package testplugin

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// Startは各テストプラグインに依存しない共通的な挙動を実装する高階関数です
// specの設定に基づき、引数に渡されたテスト関数を実行ループします
func Start(
	ctx context.Context,
	logger *slog.Logger,
	spec *v1.TestPluginSpec,
	f func(ctx context.Context, logger *slog.Logger, spec *v1.TestPluginSpec) error,
) error {
	timeoutSeconds := spec.TimeoutSeconds
	if timeoutSeconds == 0 {
		// とてつもなく大きい値を入れて、実質一生動く、というので実装を簡素化
		timeoutSeconds = 60 * 60 * 24 * 7 * 4 * 12
	}

	timeoutTicker := time.NewTicker(time.Duration(timeoutSeconds) * time.Second)
	defer timeoutTicker.Stop()

	// MinConcectiveSuccessCountは常に1にする
	if spec.MinConsecutiveSuccessCount == 0 {
		spec.MinConsecutiveSuccessCount = 1
	}

	// 成功したらtrue, 失敗したらfalseが入る
	// リングバッファのようにしてメモリ使用量を一定の上界で押し付けようとしたが、
	// リングバッファをその場で実装するとロジックが複雑になりバグを生むのでここでは無限のヒストリが使えることにする
	testFnHistory := make([]error, 0)

	for {
		select {
		case <-timeoutTicker.C:
			return fmt.Errorf("!!! timeout elapsed; aborting test")
		default:
			err := f(ctx, logger, spec)

			testFnHistory = append(testFnHistory, err)

			// 設定されているときだけチェックする
			// 常に0ではないのでチェックしない
			if minConcectiveSuccessReached(testFnHistory, spec.MinConsecutiveSuccessCount) {
				return nil
			}
			if spec.MinConsecutiveFailureCount != 0 && minConcectiveFailureReached(testFnHistory, spec.MinConsecutiveFailureCount) {
				return fmt.Errorf("aborting test after %d consecutive failures (last error: \"%s\")", spec.MinConsecutiveFailureCount, testFnHistory[len(testFnHistory)-1].Error())
			}

			if spec.IntervalSeconds != 0 {
				time.Sleep(time.Duration(spec.IntervalSeconds) * time.Second)
			}
		}
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
