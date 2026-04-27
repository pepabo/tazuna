//go:build e2e

package helpers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

// waitForResourcePollInterval は WaitForResource のポーリング間隔。
// kube-apiserver 負荷とテスト所要時間のバランスで 500ms。
// CleanupNamespace と同じ値に揃えている。
const waitForResourcePollInterval = 500 * time.Millisecond

// WaitForResource は ns/name で指定したリソースがクラスタ上に存在するように
// なるまで dynamic client でポーリングし、timeout 内に観測できなければ Fail する。
//
// ADR005「testplugin と同様の wait semantics」要件の最小実装。GVR ベースで
// 任意リソース (CRD 含む) を観測できるため、Deployment / Job / TazunaHint など
// 種別を選ばず使える。
//
// 動作:
//  1. wait.PollUntilContextTimeout で Get を timeout 内に成功させる
//  2. NotFound はリトライ。それ以外のエラーは即終了して Fail
//  3. timeout 経過時は Fail
//
// 「sleep して assert は禁止」(ADR005) に従い、待機は wait.PollUntilContextTimeout
// で統一する。Gomega Eventually は helper 内では使わず、明示 Fail で伝搬を確実にする。
//
// シグネチャは ADR005 に揃えて (gvr, ns, name, timeout) を取る。dynamic client は
// 呼び出し側で 1 回作って使い回す前提 (CleanupNamespace と同じパターン)。
func WaitForResource(
	ctx context.Context,
	dyn dynamic.Interface,
	gvr schema.GroupVersionResource,
	ns, name string,
	timeout time.Duration,
) {
	GinkgoHelper()

	waitErr := wait.PollUntilContextTimeout(ctx, waitForResourcePollInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, getErr := dyn.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
			if getErr == nil {
				return true, nil
			}
			if apierrors.IsNotFound(getErr) {
				return false, nil
			}
			return false, getErr
		},
	)
	if waitErr != nil {
		Fail(fmt.Sprintf(
			"WaitForResource: %s %s/%s did not appear within %s: %v",
			gvr.String(), ns, name, timeout, waitErr,
		))
	}
}

// WaitForResourceAbsent は ns/name で指定したリソースがクラスタ上から消えるまで
// dynamic client でポーリングし、timeout 内に消失を観測できなければ Fail する。
//
// WaitForResource の対称形。主な用途は `tazuna destroy` 後の収束観測で、
// state ConfigMap 経由で削除されたリソースが kube-apiserver から実際に
// 消えるまで待つ。Get が NotFound を返した時点で成功扱いとする。
//
// 動作:
//  1. wait.PollUntilContextTimeout で Get が NotFound を返すまで待機
//  2. NotFound 以外のエラーは即終了して Fail
//  3. timeout 経過時は Fail
//
// ポーリング間隔は WaitForResource と共通の waitForResourcePollInterval (500ms)。
// ADR005「sleep して assert は禁止」に従い、明示的な PollUntilContextTimeout を使う。
func WaitForResourceAbsent(
	ctx context.Context,
	dyn dynamic.Interface,
	gvr schema.GroupVersionResource,
	ns, name string,
	timeout time.Duration,
) {
	GinkgoHelper()

	waitErr := wait.PollUntilContextTimeout(ctx, waitForResourcePollInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, getErr := dyn.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
			if getErr == nil {
				return false, nil
			}
			if apierrors.IsNotFound(getErr) {
				return true, nil
			}
			return false, getErr
		},
	)
	if waitErr != nil {
		Fail(fmt.Sprintf(
			"WaitForResourceAbsent: %s %s/%s did not disappear within %s: %v",
			gvr.String(), ns, name, timeout, waitErr,
		))
	}
}
