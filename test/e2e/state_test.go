//go:build e2e

package e2e_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/pepabo/tazuna/test/e2e/helpers"
)

// `tazuna apply --sync` / `tazuna state list` / `tazuna state diff` の e2e シナリオ。
// state コマンドの E2E 移植。
// （旧 `state sync` は `apply --sync` に統合されたため、それに追従している）
//
// 検証内容:
//  1. 専用 namespace tazuna-e2e-state を毎回削除→再作成 (冪等な掃除)
//  2. `tazuna apply --sync` で nginx Deployment を作成・state を永続化し、出現を WaitForResource で観測
//  3. state list で manifest 名とリソース情報が表示されること
//  4. state diff で apply 直後は差分なし ("No changes detected.") であること
//  5. apply 直後の `apply --sync` は冪等で、同期対象リソースが無いこと

const (
	stateNamespace   = "tazuna-e2e-state"
	stateDeployment  = "nginx"
	stateWaitTimeout = 60 * time.Second
)

var _ = Describe("tazuna state (minimal)", func() {
	var (
		ctx     context.Context
		fixture string
	)

	BeforeEach(func() {
		ctx = context.Background()

		By("deleting and recreating namespace " + stateNamespace)
		kubeClient := helpers.NewKubeClient()
		helpers.CleanupNamespace(ctx, kubeClient, stateNamespace)

		By("deleting the state ConfigMap from the previous test")
		_ = kubeClient.CoreV1().ConfigMaps("tazuna").Delete(ctx, "tazuna-state-state-minimal-nginx", metav1.DeleteOptions{})

		By("resolving the state-minimal fixture path")
		fixture = fixturePath("state-minimal", "tazuna.yaml")

		By("creating the nginx Deployment and persisting state via tazuna apply --sync")
		stdout, stderr, err := helpers.RunTazuna("apply", "--sync", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna apply --sync failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("waiting for the nginx Deployment to appear in the cluster")
		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		helpers.WaitForResource(ctx, dyn, gvr, stateNamespace, stateDeployment, stateWaitTimeout)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			helpers.DumpClusterState(ctx, helpers.NewKubeClient())
		}
	})

	It("displays managed resources via `tazuna state list`", func() {
		By("running tazuna state list")
		stdout, stderr, err := helpers.RunTazuna("state", "list", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna state list failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("verifying that stdout contains the manifest name")
		Expect(stdout).To(ContainSubstring("state-minimal-nginx"))

		By("verifying that stdout contains the Deployment resource information")
		Expect(stdout).To(ContainSubstring("apps/v1/Deployment"))
		Expect(stdout).To(ContainSubstring(stateNamespace))
		Expect(stdout).To(ContainSubstring(stateDeployment))
	})

	It("shows no diff right after apply via `tazuna state diff`", func() {
		By("running tazuna state diff")
		stdout, stderr, err := helpers.RunTazuna("state", "diff", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna state diff failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("verifying that stdout contains 'No changes detected.'")
		Expect(stdout).To(ContainSubstring("No changes detected."))
	})

	It("performs no sync right after apply via `tazuna apply --sync`", func() {
		By("running tazuna apply --sync again")
		stdout, stderr, err := helpers.RunTazuna("apply", "--sync", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna apply --sync failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("verifying that no resource was synced (idempotent re-apply)")
		// 差分が無いとき SyncManifest は "synced resource" を一切ログしない。
		// これが旧 `state sync` の "No changes to sync." に相当する。
		Expect(stderr).NotTo(ContainSubstring("synced resource"))
	})

	It("detects a diff after a manifest change via `tazuna state diff`", func() {
		By("resolving the state-modified fixture path (replicas: 2)")
		modified := fixturePath("state-modified", "tazuna.yaml")

		By("running tazuna state diff against the modified fixture")
		stdout, stderr, err := helpers.RunTazuna("state", "diff", "-f", modified)
		Expect(err).NotTo(HaveOccurred(), "tazuna state diff failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("verifying that stdout contains the manifest name and a modified diff")
		Expect(stdout).To(ContainSubstring("state-minimal-nginx"))
		Expect(stdout).To(ContainSubstring("modified"))
	})
})
