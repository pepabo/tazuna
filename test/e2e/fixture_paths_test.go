//go:build e2e

package e2e_test

import (
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
)

// fixturePath は test/e2e/testdata 配下の絶対パスを構築する。
// 引数は testdata/ 以下のパス要素 (例: "kustomize-minimal", "tazuna.yaml")。
//
// runtime.Caller(0) はこの fixture_paths_test.go 自身の絶対パスを返す。
// 他テストファイルから呼び出されても e2eDir は test/e2e 固定なので、
// 各 test ファイルに重複していた *FixturePath() resolver を 1 箇所に集約できる。
//
// go test の cwd (= package ディレクトリ) 依存を避けるための仕組みは
// 各 resolver と同じ。tazuna 側の path 解決
// (filepath.Dir(tazunaYAMLPath) からの相対) はそのまま動く。
func fixturePath(parts ...string) string {
	GinkgoHelper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		Fail("runtime.Caller failed: cannot resolve test/e2e dir")
	}
	e2eDir := filepath.Dir(file)
	elems := append([]string{e2eDir, "testdata"}, parts...)
	return filepath.Clean(filepath.Join(elems...))
}
