//go:build e2e

package e2e_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/pepabo/tazuna/test/e2e/helpers"
)

// kustomize manager の最小 e2e シナリオ。ゴール
// 「最小 1 シナリオ (kustomize apply のみ) で CI を通す」を満たす最初のスペック。
//
// 検証内容:
//  1. 専用 namespace tazuna-e2e-kustomize を毎回削除→再作成 (冪等な掃除)
//  2. ビルド済 ./tazuna apply -f testdata/kustomize-minimal/tazuna.yaml が成功
//  3. apps/v1/Deployment tazuna-e2e-kustomize/nginx が timeout 内に observe される
//
// 失敗時は AfterEach で DumpClusterState を呼び GinkgoWriter に診断情報を残す。
const (
	kustomizeNamespace   = "tazuna-e2e-kustomize"
	kustomizeDeployment  = "nginx"
	kustomizeWaitTimeout = 60 * time.Second
)

var _ = Describe("kustomize manager (minimal)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		By("deleting and recreating namespace " + kustomizeNamespace)
		client := helpers.NewKubeClient()
		helpers.CleanupNamespace(ctx, client, kustomizeNamespace)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			helpers.DumpClusterState(ctx, helpers.NewKubeClient())
		}
	})

	It("creates the nginx Deployment via `tazuna apply`", func() {
		By("resolving the kustomize-minimal fixture path")
		fixture := fixturePath("kustomize-minimal", "tazuna.yaml")

		By("running tazuna apply")
		stdout, stderr, err := helpers.RunTazuna("apply", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna apply failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("waiting for the nginx Deployment to appear in the cluster")
		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		helpers.WaitForResource(ctx, dyn, gvr, kustomizeNamespace, kustomizeDeployment, kustomizeWaitTimeout)
	})
})
