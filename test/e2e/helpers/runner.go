//go:build e2e

// Package helpers は test/e2e/ 配下から共有される E2E テスト用ユーティリティを提供する。
//
// 3層テストピラミッドの E2E 層で利用される。ビルド済みの ./tazuna
// バイナリを exec.Command で叩いて assertion を行う。Ginkgo v2 + Gomega 前提で
// 失敗時は Ginkgo の Fail を呼ぶ。
package helpers

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
)

// repoRoot は本ファイルの位置からリポジトリルートを解決する。
// test/e2e/helpers/runner.go から3階層上がリポジトリルート。
func repoRoot() string {
	GinkgoHelper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		Fail("runtime.Caller failed: cannot resolve repo root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// tazunaBinPath は実行する tazuna バイナリのパスを返す。
// 環境変数 TAZUNA_BIN が設定されていればそれを優先し、なければ
// リポジトリルート直下の ./tazuna を使う (`make build` の出力先)。
func tazunaBinPath() string {
	GinkgoHelper()
	if env := os.Getenv("TAZUNA_BIN"); env != "" {
		return env
	}
	return filepath.Join(repoRoot(), "tazuna")
}

// RunTazuna はビルド済み tazuna バイナリを args で起動し、
// (stdout, stderr, err) を返す。E2E テストの基本ラッパー。
//
// 呼び出し前に `make build` でバイナリが用意されていることを前提とする。
// `make test-e2e` ターゲットがこの依存を担保する。
func RunTazuna(args ...string) (string, string, error) {
	GinkgoHelper()
	return runTazunaWithDir("", args...)
}

// RunTazunaInDir は cwd を指定して tazuna バイナリを起動する。
// `manifests[].path` が tazuna.yaml ディレクトリからの相対で書かれているケースは
// `tazuna.go` の `ConvertManifestPathFromCwd` で吸収されるが、
// parallel manager の `parallel.children[].path` は同関数の対象外で、
// 子マニフェストの相対パスは `tazuna apply` 実行時の cwd を起点に解決される。
// e2e で parallel-minimal のような fixture を扱う際にこのヘルパで cwd を
// fixture ディレクトリに固定する。
func RunTazunaInDir(dir string, args ...string) (string, string, error) {
	GinkgoHelper()
	return runTazunaWithDir(dir, args...)
}

func runTazunaWithDir(dir string, args ...string) (string, string, error) {
	GinkgoHelper()
	bin := tazunaBinPath()
	if _, err := os.Stat(bin); err != nil {
		Fail("tazuna binary not found at " + bin + ": " + err.Error() + " (run `make build` first)")
	}
	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
