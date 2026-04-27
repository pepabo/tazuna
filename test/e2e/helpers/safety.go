//go:build e2e

package helpers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"k8s.io/client-go/tools/clientcmd"
)

// AllowedKubeContext は E2E スイート全体が実行を許可される唯一の
// kubeconfig current-context。Makefile devenv-create が作る KinD クラスタに
// 対応する。意図しない本番クラスタへの破壊的操作を防ぐためのセーフガード。
const AllowedKubeContext = "kind-tazuna"

// EnsureSafeKubeContext は現在の kubeconfig の current-context が
// AllowedKubeContext と一致することを検証する。一致しない場合は
// Ginkgo の Fail を即座に呼び、スイート全体を異常終了させる。
//
// BeforeSuite から呼ばれる想定。任意のスペック実行前にガードが効くため、
// kind-tazuna 以外のクラスタに対して E2E がリソースを作る事故を防げる。
func EnsureSafeKubeContext() {
	GinkgoHelper()
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	).RawConfig()
	if err != nil {
		Fail(fmt.Sprintf("E2E aborted: failed to load kubeconfig: %v", err))
	}
	if cfg.CurrentContext != AllowedKubeContext {
		Fail(fmt.Sprintf(
			"E2E aborted: current kubeconfig context is %q, but only %q is allowed for safety. "+
				"Run `make devenv-create` and `kubectl config use-context %s` before running E2E.",
			cfg.CurrentContext, AllowedKubeContext, AllowedKubeContext,
		))
	}
}
