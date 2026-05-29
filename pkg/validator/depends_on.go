package validator

import (
	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

// ValidateDependsOn は include 展開後の全 manifest に対して dependsOn の整合性を検証する。
// 検証項目:
//   - dependsOn の参照先が存在する manifest 名であること
//   - 自分自身を dependsOn に含めていないこと
//   - 循環依存が存在しないこと
//
// 同名 manifest の重複については ValidateManifestNames 側で検出済みのため、ここでは
// 重複名のチェックは行わず後勝ちで索引を構築する。
func ValidateDependsOn(manifests []v1.Manifest) error {
	if len(manifests) == 0 {
		return nil
	}

	idxByName := make(map[string]int, len(manifests))
	for i, m := range manifests {
		if m.Name != "" {
			idxByName[m.Name] = i
		}
	}

	var errs []error

	// 参照整合性と自己参照のチェック
	for _, m := range manifests {
		for _, dep := range m.DependsOn {
			if dep == m.Name {
				errs = append(errs, errors.Errorf("manifest %q depends on itself", m.Name))
				continue
			}
			if _, ok := idxByName[dep]; !ok {
				errs = append(errs, errors.Errorf("manifest %q dependsOn references unknown manifest %q", m.Name, dep))
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	// 循環依存の検出: Kahn's algorithm を流用し、最終的に処理済み数 != 総数なら循環あり
	inDegree := make([]int, len(manifests))
	dependents := make([][]int, len(manifests))
	for i, m := range manifests {
		for _, dep := range m.DependsOn {
			depIdx, ok := idxByName[dep]
			if !ok {
				continue // 参照不正は上で検出済み
			}
			dependents[depIdx] = append(dependents[depIdx], i)
			inDegree[i]++
		}
	}

	processed := 0
	for processed < len(manifests) {
		progressed := false
		for i := range manifests {
			if inDegree[i] == 0 {
				inDegree[i] = -1
				for _, d := range dependents[i] {
					if inDegree[d] > 0 {
						inDegree[d]--
					}
				}
				processed++
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}

	if processed != len(manifests) {
		return errors.New("circular dependency detected in manifest dependsOn graph")
	}

	return nil
}
