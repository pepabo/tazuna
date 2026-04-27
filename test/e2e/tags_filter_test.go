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

// --tags フィルタの e2e シナリオ。ADR005 Phase 3。
//
// 検証内容:
//  1. fixture に alpha / beta 2 つの kustomize manifest を用意
//  2. `tazuna apply --tags alpha` で alpha のみ apply
//  3. nginx-alpha Deployment が出現し、nginx-beta は存在しないことを検証
const (
	tagsFilterNamespace   = "tazuna-e2e-tags-filter"
	tagsFilterAlpha       = "nginx-alpha"
	tagsFilterBeta        = "nginx-beta"
	tagsFilterWaitTimeout = 60 * time.Second
)

var _ = Describe("--tags filter (apply)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		By("deleting and recreating namespace " + tagsFilterNamespace)
		client := helpers.NewKubeClient()
		helpers.CleanupNamespace(ctx, client, tagsFilterNamespace)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			helpers.DumpClusterState(ctx, helpers.NewKubeClient())
		}
	})

	It("applies only alpha and does not create beta when running `tazuna apply --tags alpha`", func() {
		By("resolving the tags-filter-minimal fixture path")
		fixture := fixturePath("tags-filter-minimal", "tazuna.yaml")

		By("running tazuna apply --tags alpha")
		stdout, stderr, err := helpers.RunTazuna("apply", "-f", fixture, "--tags", "alpha")
		Expect(err).NotTo(HaveOccurred(), "tazuna apply failed\nstdout=%s\nstderr=%s", stdout, stderr)

		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

		By("waiting for the nginx-alpha Deployment to appear in the cluster")
		helpers.WaitForResource(ctx, dyn, gvr, tagsFilterNamespace, tagsFilterAlpha, tagsFilterWaitTimeout)

		By("verifying that the nginx-beta Deployment does not exist")
		helpers.WaitForResourceAbsent(ctx, dyn, gvr, tagsFilterNamespace, tagsFilterBeta, 5*time.Second)
	})
})
