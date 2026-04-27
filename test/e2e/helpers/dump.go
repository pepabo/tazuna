//go:build e2e

package helpers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DumpClusterState は kube-apiserver の状態を kubectl get all -A 相当の粒度で
// GinkgoWriter に書き出す。Ginkgo は spec が失敗したときだけ GinkgoWriter の
// 内容を最終出力に含めるため、AfterEach や DeferCleanup から無条件に呼んでよい
// (成功時は静か、失敗時のみ診断情報が見える)。
//
// ADR005「速度と安定性 / 失敗時の診断情報: テスト失敗時に kubectl get all -A
// 相当の情報を stdout に dump するヘルパを必ず通す」要件の実装。
//
// dump 対象は kubectl get all のコア種別に揃える:
//   - Pods / Services
//   - Deployments / ReplicaSets / StatefulSets / DaemonSets
//   - Jobs / CronJobs
//
// kube-system は省略しない。tazuna が触る namespace の隣接状態 (CNI / coredns
// が落ちているなど) も診断に必要なため、全 namespace を対象とする。
//
// 取得失敗 (一部 API が unreachable など) は致命的にせず、エラー文字列を
// 出力に含めて続行する。dump 自体で Fail を呼ぶと本来の失敗原因が隠れるため、
// このヘルパは絶対に Fail しない方針で他ヘルパ (kube.go / wait.go) と
// 役割分担する。
func DumpClusterState(ctx context.Context, client kubernetes.Interface) {
	GinkgoHelper()

	fmt.Fprintln(GinkgoWriter, "=== DumpClusterState (equivalent to kubectl get all -A) ===")

	// Pods
	fmt.Fprintln(GinkgoWriter, "--- Pods ---")
	if pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); err != nil {
		fmt.Fprintf(GinkgoWriter, "DumpClusterState: failed to list pods: %v\n", err)
	} else {
		for _, p := range pods.Items {
			fmt.Fprintf(GinkgoWriter, "%s/%s  phase=%s  age=%s\n",
				p.Namespace, p.Name, p.Status.Phase, ageOf(p.CreationTimestamp))
		}
	}

	// Services
	fmt.Fprintln(GinkgoWriter, "--- Services ---")
	if svcs, err := client.CoreV1().Services("").List(ctx, metav1.ListOptions{}); err != nil {
		fmt.Fprintf(GinkgoWriter, "DumpClusterState: failed to list services: %v\n", err)
	} else {
		for _, s := range svcs.Items {
			fmt.Fprintf(GinkgoWriter, "%s/%s  type=%s  clusterIP=%s  age=%s\n",
				s.Namespace, s.Name, s.Spec.Type, s.Spec.ClusterIP, ageOf(s.CreationTimestamp))
		}
	}

	// Deployments
	fmt.Fprintln(GinkgoWriter, "--- Deployments ---")
	if deps, err := client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{}); err != nil {
		fmt.Fprintf(GinkgoWriter, "DumpClusterState: failed to list deployments: %v\n", err)
	} else {
		for _, d := range deps.Items {
			fmt.Fprintf(GinkgoWriter, "%s/%s  ready=%d/%d  age=%s\n",
				d.Namespace, d.Name, d.Status.ReadyReplicas, d.Status.Replicas, ageOf(d.CreationTimestamp))
		}
	}

	// ReplicaSets
	fmt.Fprintln(GinkgoWriter, "--- ReplicaSets ---")
	if rss, err := client.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{}); err != nil {
		fmt.Fprintf(GinkgoWriter, "DumpClusterState: failed to list replicasets: %v\n", err)
	} else {
		for _, r := range rss.Items {
			fmt.Fprintf(GinkgoWriter, "%s/%s  ready=%d/%d  age=%s\n",
				r.Namespace, r.Name, r.Status.ReadyReplicas, r.Status.Replicas, ageOf(r.CreationTimestamp))
		}
	}

	// StatefulSets
	fmt.Fprintln(GinkgoWriter, "--- StatefulSets ---")
	if sts, err := client.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{}); err != nil {
		fmt.Fprintf(GinkgoWriter, "DumpClusterState: failed to list statefulsets: %v\n", err)
	} else {
		for _, s := range sts.Items {
			fmt.Fprintf(GinkgoWriter, "%s/%s  ready=%d/%d  age=%s\n",
				s.Namespace, s.Name, s.Status.ReadyReplicas, s.Status.Replicas, ageOf(s.CreationTimestamp))
		}
	}

	// DaemonSets
	fmt.Fprintln(GinkgoWriter, "--- DaemonSets ---")
	if dss, err := client.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{}); err != nil {
		fmt.Fprintf(GinkgoWriter, "DumpClusterState: failed to list daemonsets: %v\n", err)
	} else {
		for _, d := range dss.Items {
			fmt.Fprintf(GinkgoWriter, "%s/%s  ready=%d/%d  age=%s\n",
				d.Namespace, d.Name, d.Status.NumberReady, d.Status.DesiredNumberScheduled, ageOf(d.CreationTimestamp))
		}
	}

	// Jobs
	fmt.Fprintln(GinkgoWriter, "--- Jobs ---")
	if jobs, err := client.BatchV1().Jobs("").List(ctx, metav1.ListOptions{}); err != nil {
		fmt.Fprintf(GinkgoWriter, "DumpClusterState: failed to list jobs: %v\n", err)
	} else {
		for _, j := range jobs.Items {
			fmt.Fprintf(GinkgoWriter, "%s/%s  succeeded=%d  failed=%d  age=%s\n",
				j.Namespace, j.Name, j.Status.Succeeded, j.Status.Failed, ageOf(j.CreationTimestamp))
		}
	}

	// CronJobs
	fmt.Fprintln(GinkgoWriter, "--- CronJobs ---")
	if cjs, err := client.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{}); err != nil {
		fmt.Fprintf(GinkgoWriter, "DumpClusterState: failed to list cronjobs: %v\n", err)
	} else {
		for _, c := range cjs.Items {
			fmt.Fprintf(GinkgoWriter, "%s/%s  schedule=%q  active=%d  age=%s\n",
				c.Namespace, c.Name, c.Spec.Schedule, len(c.Status.Active), ageOf(c.CreationTimestamp))
		}
	}

	fmt.Fprintln(GinkgoWriter, "=== DumpClusterState end ===")
}

// ageOf は CreationTimestamp からの経過時間を秒精度の短い文字列で返す。
// kubectl get の AGE カラムと同じ用途で、診断時に「いつ作られたか」を素早く
// 把握できれば十分なため Round(time.Second) で粗くして読みやすさを優先する。
func ageOf(t metav1.Time) string {
	if t.IsZero() {
		return "<unknown>"
	}
	return time.Since(t.Time).Round(time.Second).String()
}
