package runner

import (
	"context"
	"os/exec"
	"strings"

	"github.com/pepabo/tazuna/pkg/state"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// buildUnstructuredFromStateKey はステートキーからunstructuredオブジェクトを構築する。
// removed リソースを削除する際、state key から GVK/namespace/name を復元するために使う。
func buildUnstructuredFromStateKey(keyStr string) (*unstructured.Unstructured, error) {
	parsed, err := state.ParseStateKey(keyStr)
	if err != nil {
		return nil, err
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   parsed.Group,
		Version: parsed.Version,
		Kind:    parsed.Kind,
	})
	obj.SetName(parsed.Name)
	if parsed.Namespace != "" {
		obj.SetNamespace(parsed.Namespace)
	}

	return obj, nil
}

// getGitCommitHash は現在のgit commit hashを取得する。
// state metadata に「どのコミット時点で同期されたか」を残すために使う。
// 取得に失敗した場合は空文字列を返す (state 保存はベストエフォート)。
func getGitCommitHash(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
