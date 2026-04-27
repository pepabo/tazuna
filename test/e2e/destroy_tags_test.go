//go:build e2e

package e2e_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/pepabo/tazuna/test/e2e/helpers"
)

// `tazuna destroy --tags` の e2e シナリオ。ADR005 Phase 3。
//
// 検証内容:
//  1. tags-filter-minimal fixture で alpha / beta 両方を apply
//  2. `tazuna destroy --force --tags alpha` で alpha のみ削除
//  3. nginx-alpha が消失し、nginx-beta は残存することを検証

const (
	destroyTagsNamespace   = "tazuna-e2e-tags-filter"
	destroyTagsAlpha       = "nginx-alpha"
	destroyTagsBeta        = "nginx-beta"
	destroyTagsWaitTimeout = 60 * time.Second
)

var _ = Describe("tazuna destroy --tags (filter)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		By("deleting and recreating namespace " + destroyTagsNamespace)
		client := helpers.NewKubeClient()
		helpers.CleanupNamespace(ctx, client, destroyTagsNamespace)

		By("applying both alpha and beta from the tags-filter-minimal fixture")
		fixture := fixturePath("tags-filter-minimal", "tazuna.yaml")
		stdout, stderr, err := helpers.RunTazuna("apply", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna apply failed\nstdout=%s\nstderr=%s", stdout, stderr)

		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

		By("waiting for the nginx-alpha Deployment to appear in the cluster")
		helpers.WaitForResource(ctx, dyn, gvr, destroyTagsNamespace, destroyTagsAlpha, destroyTagsWaitTimeout)

		By("waiting for the nginx-beta Deployment to appear in the cluster")
		helpers.WaitForResource(ctx, dyn, gvr, destroyTagsNamespace, destroyTagsBeta, destroyTagsWaitTimeout)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			helpers.DumpClusterState(ctx, helpers.NewKubeClient())
		}
	})

	It("destroys only alpha and leaves beta when running `tazuna destroy --force --tags alpha`", func() {
		By("resolving the tags-filter-minimal fixture path")
		fixture := fixturePath("tags-filter-minimal", "tazuna.yaml")

		By("setting TAZUNA_DESTROY_EXECUTABLE=true")
		Expect(os.Setenv("TAZUNA_DESTROY_EXECUTABLE", "true")).To(Succeed())
		DeferCleanup(func() {
			Expect(os.Unsetenv("TAZUNA_DESTROY_EXECUTABLE")).To(Succeed())
		})

		By("running tazuna destroy --force --tags alpha")
		stdout, stderr, err := helpers.RunTazuna("destroy", "-f", fixture, "--force", "--tags", "alpha")
		Expect(err).NotTo(HaveOccurred(), "tazuna destroy failed\nstdout=%s\nstderr=%s", stdout, stderr)

		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

		By("waiting for the nginx-alpha Deployment to disappear from the cluster")
		helpers.WaitForResourceAbsent(ctx, dyn, gvr, destroyTagsNamespace, destroyTagsAlpha, destroyTagsWaitTimeout)

		By("verifying that the nginx-beta Deployment remains in the cluster")
		helpers.WaitForResource(ctx, dyn, gvr, destroyTagsNamespace, destroyTagsBeta, 5*time.Second)
	})
})
