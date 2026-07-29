//go:build e2e

package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pepabo/tazuna/test/e2e/helpers"
)

// `tazuna build` の最小 e2e サニティシナリオ。
//
// kustomize-minimal fixture を再利用し、`./tazuna build -f ...` の標準出力に
// kustomize で生成された Deployment manifest が含まれることを assert する。
// クラスタを mutate しないため namespace 掃除や DumpClusterState は不要。
//
// build の RunE は ctrl.GetConfig() で kubeconfig を要求するが、
// e2e_suite_test.go の BeforeSuite で EnsureSafeKubeContext() により
// current-context が kind-tazuna であることが保証されている。

var _ = Describe("tazuna build (kustomize minimal)", func() {
	It("emits the Deployment manifest to stdout from kustomize-minimal tazuna.yaml", func() {
		By("resolving the kustomize-minimal fixture path")
		fixture := fixturePath("kustomize-minimal", "tazuna.yaml")

		By("running tazuna build")
		stdout, stderr, err := helpers.RunTazuna("build", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna build failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("verifying that stdout contains the Deployment manifest")
		Expect(stdout).To(ContainSubstring("kind: Deployment"))
		Expect(stdout).To(ContainSubstring("name: nginx"))
		Expect(stdout).To(ContainSubstring("namespace: tazuna-e2e-kustomize"))
	})
})
