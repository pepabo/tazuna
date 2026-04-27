package state

import (
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StateKey はステートエントリのキーを表す構造体
type StateKey struct {
	ManifestName string
	Group        string
	Version      string
	Kind         string
	Namespace    string // クラスタスコープリソースの場合は空文字
	Name         string
}

// NewStateKey はK8sオブジェクトからステートキーを生成する
func NewStateKey(manifestName string, obj client.Object) StateKey {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return StateKey{
		ManifestName: manifestName,
		Group:        gvk.Group,
		Version:      gvk.Version,
		Kind:         gvk.Kind,
		Namespace:    obj.GetNamespace(),
		Name:         obj.GetName(),
	}
}

// String はステートキーを文字列に変換する。
// namespaced: {manifest}/{group}/{version}/{kind}/{namespace}/{name}
// cluster-scoped: {manifest}/{group}/{version}/{kind}/{name}
func (k StateKey) String() string {
	if k.Namespace == "" {
		return fmt.Sprintf("%s/%s/%s/%s/%s", k.ManifestName, k.Group, k.Version, k.Kind, k.Name)
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s", k.ManifestName, k.Group, k.Version, k.Kind, k.Namespace, k.Name)
}

// ParseStateKey は文字列からStateKeyをパースする。
// namespaced (6パート): {manifest}/{group}/{version}/{kind}/{namespace}/{name}
// cluster-scoped (5パート): {manifest}/{group}/{version}/{kind}/{name}
func ParseStateKey(s string) (StateKey, error) {
	parts := strings.SplitN(s, "/", 6)

	switch len(parts) {
	case 5:
		// cluster-scoped: manifest/group/version/kind/name
		return StateKey{
			ManifestName: parts[0],
			Group:        parts[1],
			Version:      parts[2],
			Kind:         parts[3],
			Name:         parts[4],
		}, nil
	case 6:
		// namespaced: manifest/group/version/kind/namespace/name
		return StateKey{
			ManifestName: parts[0],
			Group:        parts[1],
			Version:      parts[2],
			Kind:         parts[3],
			Namespace:    parts[4],
			Name:         parts[5],
		}, nil
	default:
		return StateKey{}, fmt.Errorf("invalid state key format: %q (expected 5 or 6 parts separated by '/')", s)
	}
}
