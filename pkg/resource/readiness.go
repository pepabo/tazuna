package resource

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// IsReady は unstructured 化されたオブジェクトの Ready 状態を判定する。
// 対応 Kind: Deployment, StatefulSet, DaemonSet, Pod。
// 上記以外は常に true を返す (ConfigMap, Secret, Service 等は即 Ready 扱い)。
func IsReady(obj *unstructured.Unstructured) (bool, error) {
	if obj == nil {
		return false, nil
	}
	kind := obj.GetKind()
	switch kind {
	case "Deployment":
		return IsDeploymentReady(obj)
	case "StatefulSet":
		return IsStatefulSetReady(obj)
	case "DaemonSet":
		return IsDaemonSetReady(obj)
	case "Pod":
		return IsPodReady(obj)
	default:
		// その他のリソースタイプは即座に ready とみなす
		// (ConfigMap, Secret, Service など)
		return true, nil
	}
}

// IsDeploymentReady は Deployment が Ready かどうかを確認します。
func IsDeploymentReady(u *unstructured.Unstructured) (bool, error) {
	// spec.replicas が 0 の場合は即座に ready とみなす
	specReplicas, _, _ := unstructured.NestedInt64(u.Object, "spec", "replicas")
	if specReplicas == 0 {
		return true, nil
	}

	readyReplicas, _, _ := unstructured.NestedInt64(u.Object, "status", "readyReplicas")
	availableReplicas, _, _ := unstructured.NestedInt64(u.Object, "status", "availableReplicas")
	replicas, _, _ := unstructured.NestedInt64(u.Object, "status", "replicas")

	return readyReplicas == replicas && availableReplicas == replicas && replicas > 0, nil
}

// IsStatefulSetReady は StatefulSet が Ready かどうかを確認します。
func IsStatefulSetReady(u *unstructured.Unstructured) (bool, error) {
	// spec.replicas が 0 の場合は即座に ready とみなす
	specReplicas, _, _ := unstructured.NestedInt64(u.Object, "spec", "replicas")
	if specReplicas == 0 {
		return true, nil
	}

	readyReplicas, _, _ := unstructured.NestedInt64(u.Object, "status", "readyReplicas")
	replicas, _, _ := unstructured.NestedInt64(u.Object, "status", "replicas")

	return readyReplicas == replicas && replicas > 0, nil
}

// IsDaemonSetReady は DaemonSet が Ready かどうかを確認します。
func IsDaemonSetReady(u *unstructured.Unstructured) (bool, error) {
	numberReady, _, _ := unstructured.NestedInt64(u.Object, "status", "numberReady")
	desiredNumberScheduled, _, _ := unstructured.NestedInt64(u.Object, "status", "desiredNumberScheduled")

	return numberReady == desiredNumberScheduled && desiredNumberScheduled > 0, nil
}

// IsPodReady は Pod が Ready かどうかを確認します。
func IsPodReady(u *unstructured.Unstructured) (bool, error) {
	phase, _, _ := unstructured.NestedString(u.Object, "status", "phase")
	if phase != "Running" {
		return false, nil
	}

	conditions, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conditions {
		condition, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if condition["type"] == "Ready" {
			return condition["status"] == "True", nil
		}
	}

	return false, nil
}
