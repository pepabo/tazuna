//go:build e2e

package helpers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// cleanupNamespaceTimeout は CleanupNamespace が namespace 削除完了を待つ
// デフォルトタイムアウト。WaitForResource のデフォルト timeout (60 秒) の
// 基準に揃えている。これを超えるなら namespace 内のリソース GC が詰まっている
// 可能性が高く、テスト設計を見直す合図とする。
const cleanupNamespaceTimeout = 60 * time.Second

// cleanupNamespacePollInterval は削除完了ポーリングの間隔。
// kube-apiserver への負荷とテスト所要時間のバランスで 500ms。
const cleanupNamespacePollInterval = 500 * time.Millisecond

// CleanupNamespace は ns を削除→再作成して、テスト開始時にまっさらな状態にする。
//
// 「冪等な掃除: 各シナリオは開始時に対象 namespace を削除→再作成する
// (前テストの残骸に依存しない)」要件の最小実装。各 Describe / It の BeforeEach から呼ぶ。
//
// 動作:
//  1. ns を Delete (NotFound は無視)
//  2. Get が NotFound を返すまで PollUntilContextTimeout で待機 (最大 60 秒)
//  3. 同名 namespace を Create
//
// 失敗時は Ginkgo Fail を呼ぶ。AllowedKubeContext (kind-tazuna) のセーフガードは
// BeforeSuite で済んでいるため、ここでは呼ばない。
//
// sleep して assert するのは禁止のため待機は wait.PollUntilContextTimeout
// を使う。Gomega Eventually を helper 関数内で使うと assertion failure の伝搬が
// 不確実になるため避け、明示的に Fail を呼ぶ方針とする。
func CleanupNamespace(ctx context.Context, client kubernetes.Interface, ns string) {
	GinkgoHelper()

	err := client.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		Fail(fmt.Sprintf("CleanupNamespace: failed to delete namespace %q: %v", ns, err))
	}

	waitErr := wait.PollUntilContextTimeout(ctx, cleanupNamespacePollInterval, cleanupNamespaceTimeout, true,
		func(ctx context.Context) (bool, error) {
			_, getErr := client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
			if apierrors.IsNotFound(getErr) {
				return true, nil
			}
			if getErr != nil {
				return false, getErr
			}
			return false, nil
		},
	)
	if waitErr != nil {
		Fail(fmt.Sprintf("CleanupNamespace: namespace %q did not finish terminating within %s: %v", ns, cleanupNamespaceTimeout, waitErr))
	}

	_, createErr := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	if createErr != nil {
		Fail(fmt.Sprintf("CleanupNamespace: failed to create namespace %q: %v", ns, createErr))
	}
}
