//go:build e2e

package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pepabo/tazuna/test/e2e/helpers"
)

// `tazuna check` の e2e シナリオ。ADR005 Phase 3。
//
// 検証内容:
//  1. 正常な tazuna.yaml に対して exit code 0 + "ok" 出力
//  2. 不正な tazuna.yaml (name 未設定) に対して非ゼロ exit code

var _ = Describe("tazuna check", func() {
	It("prints 'ok' for a valid tazuna.yaml", func() {
		By("resolving the kustomize-minimal fixture (valid) path")
		fixture := fixturePath("kustomize-minimal", "tazuna.yaml")

		By("running tazuna check")
		stdout, stderr, err := helpers.RunTazuna("check", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna check failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("verifying that stdout contains 'ok'")
		Expect(stdout).To(ContainSubstring("ok"))
	})

	It("returns a non-zero exit code for an invalid tazuna.yaml (missing name)", func() {
		By("resolving the check-invalid fixture path")
		fixture := fixturePath("check-invalid", "tazuna.yaml")

		By("running tazuna check")
		_, _, err := helpers.RunTazuna("check", "-f", fixture)
		Expect(err).To(HaveOccurred(), "tazuna check should fail for invalid tazuna.yaml")
	})
})
