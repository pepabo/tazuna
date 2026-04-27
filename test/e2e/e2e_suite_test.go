//go:build e2e

package e2e_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pepabo/tazuna/test/e2e/helpers"
)

// TestE2E は test/e2e/ 配下の Ginkgo スペック群のエントリポイント。
// `go test -tags=e2e ./test/e2e/...` でこの関数経由でスペックが実行される。
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "tazuna E2E Suite")
}

// BeforeSuite は全スペック実行前のセーフティガード。kubeconfig current-context が
// kind-tazuna でない場合、ここで Fail してスイート全体を即異常終了させる。
// 意図しない本番クラスタへの apply 事故を防ぐため、必須のチェック。
var _ = BeforeSuite(func() {
	helpers.EnsureSafeKubeContext()
})
