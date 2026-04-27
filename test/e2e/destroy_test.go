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

// `tazuna destroy` の最小 e2e シナリオ。ADR005 Phase 2 の追加分。
//
// 検証内容:
//  1. 専用 namespace tazuna-e2e-destroy を毎回削除→再作成 (冪等な掃除)
//  2. ./tazuna apply で nginx Deployment を作成し、出現を WaitForResource で観測
//  3. TAZUNA_DESTROY_EXECUTABLE=true を立てて ./tazuna destroy --force を実行
//  4. WaitForResourceAbsent で Deployment 消失を観測
//
// destroy は以下の 2 段ガードを通る必要がある (cmd/destroy.go):
//   - --force: 対話プロンプトをスキップ
//   - 環境変数 TAZUNA_DESTROY_EXECUTABLE=true: 実行を許可
//
// fixture は kustomize-minimal とは別に destroy-minimal を用意している。
// 理由: kustomize-minimal の Deployment は namespace を hardcode しており、
// 同 namespace を共有すると ADR005「namespace 単位で衝突を避ける」原則に反し、
// 将来の `--procs > 1` での並列化と相性が悪いため。

const (
	destroyNamespace   = "tazuna-e2e-destroy"
	destroyDeployment  = "nginx"
	destroyWaitTimeout = 60 * time.Second
)

var _ = Describe("tazuna destroy (minimal)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		By("deleting and recreating namespace " + destroyNamespace)
		client := helpers.NewKubeClient()
		helpers.CleanupNamespace(ctx, client, destroyNamespace)

		By("creating the destroy target nginx Deployment via tazuna apply")
		fixture := fixturePath("destroy-minimal", "tazuna.yaml")
		stdout, stderr, err := helpers.RunTazuna("apply", "-f", fixture)
		Expect(err).NotTo(HaveOccurred(), "tazuna apply failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("waiting for the nginx Deployment to appear in the cluster")
		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		helpers.WaitForResource(ctx, dyn, gvr, destroyNamespace, destroyDeployment, destroyWaitTimeout)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			helpers.DumpClusterState(ctx, helpers.NewKubeClient())
		}
	})

	It("deletes the nginx Deployment via `tazuna destroy --force`", func() {
		By("resolving the destroy-minimal fixture path")
		fixture := fixturePath("destroy-minimal", "tazuna.yaml")

		By("setting TAZUNA_DESTROY_EXECUTABLE=true")
		// destroy は TAZUNA_DESTROY_EXECUTABLE=true がなければ no-op で抜けるため、
		// 子プロセスへ env を伝搬させる目的で親プロセスに一時的に設定する。
		// DeferCleanup で必ず元に戻す。
		Expect(os.Setenv("TAZUNA_DESTROY_EXECUTABLE", "true")).To(Succeed())
		DeferCleanup(func() {
			Expect(os.Unsetenv("TAZUNA_DESTROY_EXECUTABLE")).To(Succeed())
		})

		By("running tazuna destroy --force")
		stdout, stderr, err := helpers.RunTazuna("destroy", "-f", fixture, "--force")
		Expect(err).NotTo(HaveOccurred(), "tazuna destroy failed\nstdout=%s\nstderr=%s", stdout, stderr)

		By("waiting for the nginx Deployment to disappear from the cluster")
		dyn := helpers.NewDynamicClient()
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		helpers.WaitForResourceAbsent(ctx, dyn, gvr, destroyNamespace, destroyDeployment, destroyWaitTimeout)
	})
})
