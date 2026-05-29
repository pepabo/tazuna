package runner

import (
	"sort"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

// Layer は同時並列実行可能なマニフェストのグループ (依存深度ごと)。
// 同一層に属するマニフェストは互いに依存していないため並列適用できる。
type Layer []v1.Manifest

// HasAnyDependsOn は manifests のうち1つでも dependsOn を持つかを返す。
// 1つも持たない場合は後方互換のために宣言順の単線実行 (各層1 manifest) を行う。
func HasAnyDependsOn(manifests []v1.Manifest) bool {
	for _, m := range manifests {
		if len(m.DependsOn) > 0 {
			return true
		}
	}
	return false
}

// ResolveDependencyOrder は manifests を dependsOn に基づいてトポロジカルソートし、
// 層ごとに並列実行可能な配列を返す。各層内のマニフェストは元の宣言順を保持する。
//
// 後方互換のため、dependsOn が 1 つも使われていなければ宣言順 1 manifest = 1 層と
// なるよう各 manifest を独立した層に分割する (= 従来の順次実行と同じ挙動)。
//
// 1 つでも dependsOn が使われていれば DAG モードに切り替わり、Kahn's algorithm に
// よるトポロジカルソートを行う。依存深度ごとに層が形成されるため、同一層に複数
// manifest が存在する場合は並列適用が可能になる。
//
// エラー:
//   - dependsOn が存在しない manifest 名を参照した場合
//   - 自分自身を dependsOn に含めた場合
//   - 循環依存が検出された場合
func ResolveDependencyOrder(manifests []v1.Manifest) ([]Layer, error) {
	if len(manifests) == 0 {
		return nil, nil
	}

	// dependsOn が一切使われていなければ各 manifest を独立した層に分け、
	// 既存の宣言順順次実行と同じ挙動を維持する。
	if !HasAnyDependsOn(manifests) {
		layers := make([]Layer, 0, len(manifests))
		for _, m := range manifests {
			layers = append(layers, Layer{m})
		}
		return layers, nil
	}

	// 名前ベースの索引を作成する。Name 同士の重複検出は manifest_name の
	// バリデーションに委ねるため、ここでは後勝ちで構わない。
	idxByName := make(map[string]int, len(manifests))
	for i, m := range manifests {
		if m.Name != "" {
			idxByName[m.Name] = i
		}
	}

	// 入次数と隣接リスト (依存元 -> 依存先のスライス) を構築する。
	inDegree := make([]int, len(manifests))
	dependents := make([][]int, len(manifests))

	for i, m := range manifests {
		for _, dep := range m.DependsOn {
			if dep == m.Name {
				return nil, errors.Errorf("manifest %q depends on itself", m.Name)
			}
			depIdx, ok := idxByName[dep]
			if !ok {
				return nil, errors.Errorf("dependsOn references unknown manifest %q", dep)
			}
			dependents[depIdx] = append(dependents[depIdx], i)
			inDegree[i]++
		}
	}

	// Kahn's algorithm: 各イテレーションで「入次数 0」の集合を 1 つの層とする。
	// 層内のマニフェストは元の宣言順 (= インデックスの昇順) でソートし安定性を保つ。
	var layers []Layer
	processed := 0
	for processed < len(manifests) {
		var current []int
		for i := 0; i < len(manifests); i++ {
			if inDegree[i] == 0 {
				current = append(current, i)
			}
		}
		if len(current) == 0 {
			break
		}

		sort.Ints(current)

		layer := make(Layer, 0, len(current))
		for _, i := range current {
			layer = append(layer, manifests[i])
			// この層に入ったマニフェストは以降の判定対象から外す。
			// in-degree を -1 にすることで以降の探索でスキップさせる。
			inDegree[i] = -1
		}
		layers = append(layers, layer)
		processed += len(current)

		// 直前の層に含めたマニフェストに依存していたマニフェストの入次数を減らす。
		for _, i := range current {
			for _, dep := range dependents[i] {
				if inDegree[dep] > 0 {
					inDegree[dep]--
				}
			}
		}
	}

	if processed != len(manifests) {
		return nil, errors.New("circular dependency detected")
	}

	return layers, nil
}
