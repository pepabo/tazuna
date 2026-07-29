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

// testplugin WaitUntil の複雑 CEL 条件を検証する e2e シナリオ。
//
// 検証内容:
//  1. 複合 && 条件: readyReplicas == 1 && labels["app"] == "nginx" && size(containers) > 0
//  2. has() ガード付き条件: has(object.status.availableReplicas) && availableReplicas >= 1
//  3. tazuna apply の exit code 0 = 複雑 CEL 式が実クラスタで評価・パスされた証明
const (
	celNamespace   = "tazuna-e2e-testplugin-cel"
	celDeployment  = "nginx"
	celWaitTimeout = 60 * time.Second
)

var _ = Describe("testplugin WaitUntil (complex CEL conditions)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		By("deleting and recreating namespace " + celNamespace)
		client := helpers.NewKubeClient()
		helpers.CleanupNamespace(ctx, client, celNamespace)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			helpers.DumpClusterState(ctx, helpers.NewKubeClient())
		}
	})

	It("succeeds when `tazuna apply` evaluates compound CEL conditions (&&, labels, size, has)", func() {
		By("resolving the testplugin-cel fixture path")
		fixture := fixturePath("testplugin-cel", "tazuna.yaml")

		By("running tazuna apply (including the complex CEL testplugin)")
		stdout, stderr, err := helpers.RunTazuna("apply", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna apply failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("verifying from Go that the nginx Deployment exists in the cluster")
		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		helpers.WaitForResource(ctx, dyn, gvr, celNamespace, celDeployment, celWaitTimeout)
	})
})
