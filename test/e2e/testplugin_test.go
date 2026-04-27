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

// testplugin (WaitUntil / ExistNonExist) の最小 e2e シナリオ。
// ADR005 Phase 2「testplugin が実クラスタで動く」を検証する。
//
// 検証内容:
//  1. kustomize で nginx Deployment をデプロイ
//  2. tazuna.yaml に定義した testplugin が実クラスタ上で成功すること
//     - WaitUntil: readyReplicas == 1 を CEL 式で評価
//     - ExistNonExist (shouldExist: true): nginx Deployment が存在
//     - ExistNonExist (shouldExist: false): nonexistent-resource が不在
//  3. tazuna apply の exit code 0 = 全 testplugin パス
const (
	testpluginNamespace   = "tazuna-e2e-testplugin"
	testpluginDeployment  = "nginx"
	testpluginWaitTimeout = 60 * time.Second
)

var _ = Describe("testplugin (WaitUntil / ExistNonExist)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		By("deleting and recreating namespace " + testpluginNamespace)
		client := helpers.NewKubeClient()
		helpers.CleanupNamespace(ctx, client, testpluginNamespace)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			helpers.DumpClusterState(ctx, helpers.NewKubeClient())
		}
	})

	It("succeeds against a real cluster when `tazuna apply` runs testplugin (WaitUntil + ExistNonExist)", func() {
		By("resolving the testplugin-minimal fixture path")
		fixture := fixturePath("testplugin-minimal", "tazuna.yaml")

		By("running tazuna apply (including testplugin)")
		stdout, stderr, err := helpers.RunTazuna("apply", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna apply failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("verifying from Go that the nginx Deployment exists in the cluster")
		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		helpers.WaitForResource(ctx, dyn, gvr, testpluginNamespace, testpluginDeployment, testpluginWaitTimeout)
	})
})
