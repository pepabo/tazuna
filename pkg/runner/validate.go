package runner

import (
	"context"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/validator"
)

// warnManifestNameValidation はmanifest nameのバリデーションを実行し、
// エラーがあれば警告ログを出力する。移行期間のためエラーにはしない。
func (t *TazunaRunner) warnManifestNameValidation(ctx context.Context, tazuna v1.Tazuna) {
	allManifests := validator.CollectAllManifests(tazuna.Spec.Manifests)
	if err := validator.ValidateManifestNames(allManifests); err != nil {
		t.logger.WarnContext(ctx, "manifest name validation failed (run 'tazuna check' to see details, or 'tazuna check --fix' to auto-fix)", "error", err.Error())
	}
}
