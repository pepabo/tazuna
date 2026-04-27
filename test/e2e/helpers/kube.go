//go:build e2e

package helpers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// newRESTConfig は kubeconfig (current-context) から rest.Config を組み立てる
// 内部ヘルパ。NewKubeClient / NewDynamicClient で共有する。
//
// safety.go の EnsureSafeKubeContext と同じ NewDefaultClientConfigLoadingRules +
// 空 ConfigOverrides を使うことで「セーフティチェックが見ていた kubeconfig」と
// 「クライアントが触る kubeconfig」が一致することを保証する。
func newRESTConfig() *rest.Config {
	GinkgoHelper()
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		Fail(fmt.Sprintf("newRESTConfig: failed to build rest.Config from kubeconfig: %v", err))
	}
	return restCfg
}

// NewKubeClient はデフォルト kubeconfig (current-context) から client-go の
// clientset を生成して返す。E2E スペックから KinD クラスタの状態を観測するための
// 共通エントリポイント。
//
// 前提: BeforeSuite の EnsureSafeKubeContext によって current-context が
// AllowedKubeContext (kind-tazuna) であることが保証されている。よって本ヘルパが
// 返す client は kind-tazuna 以外のクラスタを指さない。意図しない本番クラスタへの
// 操作を防ぐためのセーフガードは safety.go 側に集約されている。
//
// kubeconfig のロード作法は safety.go の EnsureSafeKubeContext と揃えており、
// 同じ NewDefaultClientConfigLoadingRules + ConfigOverrides{} の組み合わせを使う。
// これにより「セーフティチェックが見ていた kubeconfig」と「テストが触る kubeconfig」が
// ズレないことを保証する。
//
// エラー時は Ginkgo の Fail を呼んでスペックを即座に失敗させる。テスト harness に
// 統合するため呼び出し元での err 戻り値ハンドリングは不要。
func NewKubeClient() kubernetes.Interface {
	GinkgoHelper()
	clientset, err := kubernetes.NewForConfig(newRESTConfig())
	if err != nil {
		Fail(fmt.Sprintf("NewKubeClient: failed to create clientset: %v", err))
	}
	return clientset
}

// NewDynamicClient はデフォルト kubeconfig (current-context) から dynamic client
// を生成して返す。GVR 指定で任意リソースを取得できるため、CRD を含む E2E の
// リソース観測 (WaitForResource など) のベースとなる。
//
// 前提と kubeconfig 解決ルールは NewKubeClient と同一。BeforeSuite の
// EnsureSafeKubeContext によって current-context が AllowedKubeContext
// (kind-tazuna) であることが保証されている。
//
// エラー時は Ginkgo Fail を呼んで即スペックを失敗させる。
func NewDynamicClient() dynamic.Interface {
	GinkgoHelper()
	dyn, err := dynamic.NewForConfig(newRESTConfig())
	if err != nil {
		Fail(fmt.Sprintf("NewDynamicClient: failed to create dynamic client: %v", err))
	}
	return dyn
}
