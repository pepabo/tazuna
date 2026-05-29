package runner

import (
	"context"
	"log/slog"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/validator"
)

// Check はtazuna.yamlのバリデーションを行う。
// include展開後の全manifest間でnameのバリデーションを実行する。
func (t *TazunaRunner) Check(ctx context.Context, tazuna *v1.Tazuna, tazunaYAMLPath string) error {
	if err := t.expandIncludes(ctx, tazuna, tazunaYAMLPath); err != nil {
		return err
	}

	allManifests := validator.CollectAllManifests(tazuna.Spec.Manifests)

	t.logger.DebugContext(ctx, "validating manifest names", slog.Int("totalManifests", len(allManifests)))

	if err := validator.ValidateManifestNames(allManifests); err != nil {
		return err
	}

	// dependsOn の参照整合性と循環依存をチェックする
	if err := validator.ValidateDependsOn(tazuna.Spec.Manifests); err != nil {
		return err
	}

	return nil
}

// CheckAndFix はname未設定のmanifestに自動的に名前を付与し、バリデーションを実行する。
// tazuna構造体は直接変更される。
func (t *TazunaRunner) CheckAndFix(ctx context.Context, tazuna *v1.Tazuna, tazunaYAMLPath string) error {
	if err := t.expandIncludes(ctx, tazuna, tazunaYAMLPath); err != nil {
		return err
	}

	validator.FixManifestNames(tazuna.Spec.Manifests)

	allManifests := validator.CollectAllManifests(tazuna.Spec.Manifests)

	t.logger.DebugContext(ctx, "validating manifest names after fix", slog.Int("totalManifests", len(allManifests)))

	if err := validator.ValidateManifestNames(allManifests); err != nil {
		return err
	}

	// dependsOn の参照整合性と循環依存をチェックする
	if err := validator.ValidateDependsOn(tazuna.Spec.Manifests); err != nil {
		return err
	}

	return nil
}
