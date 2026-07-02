package runner

import (
	"context"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/validator"
)

// validateManifestNames はinclude展開後のmanifest名バリデーションを行う。
//
// 重複名や不正名のまま state を書き込むと、(a) 同じ state ConfigMap への
// 後勝ち書き込みで他方のエントリが消え --sync --prune で生きているリソースが
// 誤って prune される、(b) dependsOn の辺が意図しない manifest に張られる、
// といった実害があるため、strict=true (--sync または dependsOn 使用時) は
// エラーに昇格する。それ以外は移行期間のため警告に留める。
func (t *TazunaRunner) validateManifestNames(ctx context.Context, tazuna v1.Tazuna, strict bool) error {
	allManifests := validator.CollectAllManifests(tazuna.Spec.Manifests)
	err := validator.ValidateManifestNames(allManifests)
	if err == nil {
		return nil
	}

	if strict {
		return errors.Wrap(err, "manifest name validation failed (run 'tazuna check' to see details, or 'tazuna check --fix' to auto-fix)")
	}

	t.logger.WarnContext(ctx, "manifest name validation failed (run 'tazuna check' to see details, or 'tazuna check --fix' to auto-fix)", "error", err.Error())
	return nil
}

// warnManifestNameValidation はmanifest nameのバリデーションを実行し、
// エラーがあれば警告ログを出力する。移行期間のためエラーにはしない。
func (t *TazunaRunner) warnManifestNameValidation(ctx context.Context, tazuna v1.Tazuna) {
	// 戻り値は strict=false のとき常に nil。
	_ = t.validateManifestNames(ctx, tazuna, false)
}

// anyDependsOn はいずれかのmanifestがdependsOnを使っているかを返す。
func anyDependsOn(manifests []v1.Manifest) bool {
	for i := range manifests {
		if len(manifests[i].DependsOn) > 0 {
			return true
		}
	}
	return false
}
