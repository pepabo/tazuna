//go:build e2e

package e2e_test

import (
	"context"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/pepabo/tazuna/test/e2e/helpers"
)

// parallel manager の最小 e2e シナリオ。ADR005 Phase 2 の追加分。
//
// 検証内容:
//  1. 専用 namespace tazuna-e2e-parallel を毎回削除→再作成 (冪等な掃除)
//  2. parallel-minimal/tazuna.yaml を `tazuna apply` で実行し、
//     parallel manager 配下の 2 つの kustomize 子マニフェスト
//     (kustomize-a / kustomize-b) が並列で適用されること
//  3. nginx-a / nginx-b 両 Deployment が timeout 内に observe される
//
// 注意: parallel manager の `parallel.children[].path` は ConvertManifestPathFromCwd
// の対象外で、相対パスは `tazuna apply` 実行時の cwd 起点で解決される。
// fixture 側は `./kustomize-a` / `./kustomize-b` という相対表記で書きたいので、
// helpers.RunTazunaInDir で cwd を fixture ディレクトリに固定する。

const (
	parallelNamespace   = "tazuna-e2e-parallel"
	parallelDeployA     = "nginx-a"
	parallelDeployB     = "nginx-b"
	parallelWaitTimeout = 60 * time.Second
)

var _ = Describe("parallel manager (minimal)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		By("deleting and recreating namespace " + parallelNamespace)
		client := helpers.NewKubeClient()
		helpers.CleanupNamespace(ctx, client, parallelNamespace)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			helpers.DumpClusterState(ctx, helpers.NewKubeClient())
		}
	})

	It("creates both nginx-a and nginx-b Deployments under parallel via `tazuna apply`", func() {
		By("resolving the parallel-minimal fixture path")
		fixture := fixturePath("parallel-minimal", "tazuna.yaml")
		fixtureDir := filepath.Dir(fixture)

		By("running tazuna apply with the fixture directory as cwd")
		stdout, stderr, err := helpers.RunTazunaInDir(fixtureDir, "apply", "-f", "tazuna.yaml")
		Expect(err).NotTo(HaveOccurred(), "tazuna apply failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("waiting for the nginx-a Deployment to appear in the cluster")
		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		helpers.WaitForResource(ctx, dyn, gvr, parallelNamespace, parallelDeployA, parallelWaitTimeout)

		By("waiting for the nginx-b Deployment to appear in the cluster")
		helpers.WaitForResource(ctx, dyn, gvr, parallelNamespace, parallelDeployB, parallelWaitTimeout)
	})
})
