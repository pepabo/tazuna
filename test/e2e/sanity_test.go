//go:build e2e

package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pepabo/tazuna/test/e2e/helpers"
)

// E2E ハーネス全体 (バイナリ解決 → exec → 結果取得) が正しく配線されている
// ことを検証する最小 sanity check。`tazuna --help` は KinD クラスタを必要と
// しないため、ヘルパの挙動をクラスタ依存と切り離して確認できる。
var _ = Describe("tazuna CLI sanity", func() {
	It("succeeds and prints usage when `tazuna --help` is run", func() {
		By("running tazuna --help")
		stdout, stderr, err := helpers.RunTazuna("--help")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("verifying that stdout contains the tazuna usage")
		Expect(stdout).To(ContainSubstring("tazuna"))
	})
})
