package runner_test

import (
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// manifestsNames は Layer に含まれる manifest の Name スライスを取り出すヘルパー。
func manifestsNames(layer runner.Layer) []string {
	names := make([]string, 0, len(layer))
	for _, m := range layer {
		names = append(names, m.Name)
	}
	return names
}

// TestResolveDependencyOrder_NoDependsOn は dependsOn が一切使われていない場合に
// 後方互換のため各 manifest が独立した層 (層数 = manifest 数) で返されることを縛る。
func TestResolveDependencyOrder_NoDependsOn(t *testing.T) {
	t.Parallel()

	manifests := []v1.Manifest{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}

	layers, err := runner.ResolveDependencyOrder(manifests)
	require.NoError(t, err)
	require.Len(t, layers, 3)
	assert.Equal(t, []string{"a"}, manifestsNames(layers[0]))
	assert.Equal(t, []string{"b"}, manifestsNames(layers[1]))
	assert.Equal(t, []string{"c"}, manifestsNames(layers[2]))
}

// TestResolveDependencyOrder_SimpleChain は A -> B -> C の依存関係が 3 層各 1
// マニフェストに分解されることを縛る。
func TestResolveDependencyOrder_SimpleChain(t *testing.T) {
	t.Parallel()

	manifests := []v1.Manifest{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"b"}},
	}

	layers, err := runner.ResolveDependencyOrder(manifests)
	require.NoError(t, err)
	require.Len(t, layers, 3)
	assert.Equal(t, []string{"a"}, manifestsNames(layers[0]))
	assert.Equal(t, []string{"b"}, manifestsNames(layers[1]))
	assert.Equal(t, []string{"c"}, manifestsNames(layers[2]))
}

// TestResolveDependencyOrder_Diamond は A -> {B, C} -> D のダイヤモンド型依存が
// 3 層に分解され、層 1 に B と C が宣言順で並ぶことを縛る。
func TestResolveDependencyOrder_Diamond(t *testing.T) {
	t.Parallel()

	manifests := []v1.Manifest{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"a"}},
		{Name: "d", DependsOn: []string{"b", "c"}},
	}

	layers, err := runner.ResolveDependencyOrder(manifests)
	require.NoError(t, err)
	require.Len(t, layers, 3)
	assert.Equal(t, []string{"a"}, manifestsNames(layers[0]))
	assert.Equal(t, []string{"b", "c"}, manifestsNames(layers[1]))
	assert.Equal(t, []string{"d"}, manifestsNames(layers[2]))
}

// TestResolveDependencyOrder_Parallel は同一層に複数 manifest が並ぶ場合に
// 宣言順 (元の Manifests スライスでのインデックス昇順) で並ぶことを縛る。
func TestResolveDependencyOrder_Parallel(t *testing.T) {
	t.Parallel()

	manifests := []v1.Manifest{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"a"}},
		{Name: "d", DependsOn: []string{"a"}},
	}

	layers, err := runner.ResolveDependencyOrder(manifests)
	require.NoError(t, err)
	require.Len(t, layers, 2)
	assert.Equal(t, []string{"a"}, manifestsNames(layers[0]))
	assert.Equal(t, []string{"b", "c", "d"}, manifestsNames(layers[1]))
}

// TestResolveDependencyOrder_CircularDependency は A -> B -> A の循環があるとき
// "circular dependency detected" エラーを返すことを縛る。
func TestResolveDependencyOrder_CircularDependency(t *testing.T) {
	t.Parallel()

	manifests := []v1.Manifest{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
	}

	_, err := runner.ResolveDependencyOrder(manifests)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

// TestResolveDependencyOrder_SelfReference は manifest が自分自身を dependsOn に
// 含めている場合、専用のエラーを返すことを縛る。
func TestResolveDependencyOrder_SelfReference(t *testing.T) {
	t.Parallel()

	manifests := []v1.Manifest{
		{Name: "a", DependsOn: []string{"a"}},
	}

	_, err := runner.ResolveDependencyOrder(manifests)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "depends on itself")
}

// TestResolveDependencyOrder_UnknownReference は dependsOn が存在しない manifest
// 名を参照しているときに "unknown manifest" を含むエラーを返すことを縛る。
func TestResolveDependencyOrder_UnknownReference(t *testing.T) {
	t.Parallel()

	manifests := []v1.Manifest{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"ghost"}},
	}

	_, err := runner.ResolveDependencyOrder(manifests)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown manifest")
	assert.Contains(t, err.Error(), "ghost")
}
