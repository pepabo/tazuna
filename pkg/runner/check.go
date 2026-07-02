package runner

import (
	"context"
	"log/slog"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/validator"
	"sigs.k8s.io/yaml"
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
// name付与はinclude展開前のtazuna構造体に対してのみ行われる（呼び出し元が
// この構造体をファイルへ書き戻すため、展開結果で破壊しない）。
// バリデーションはdeep copyに対してinclude展開を行ったうえで実行する。
func (t *TazunaRunner) CheckAndFix(ctx context.Context, tazuna *v1.Tazuna, tazunaYAMLPath string) error {
	validator.FixManifestNames(tazuna.Spec.Manifests)

	expanded, err := deepCopyTazuna(tazuna)
	if err != nil {
		return err
	}
	if err := t.expandIncludes(ctx, expanded, tazunaYAMLPath); err != nil {
		return err
	}

	allManifests := validator.CollectAllManifests(expanded.Spec.Manifests)

	t.logger.DebugContext(ctx, "validating manifest names after fix", slog.Int("totalManifests", len(allManifests)))

	if err := validator.ValidateManifestNames(allManifests); err != nil {
		return err
	}

	// dependsOn の参照整合性と循環依存をチェックする
	if err := validator.ValidateDependsOn(expanded.Spec.Manifests); err != nil {
		return err
	}

	return nil
}

// deepCopyTazuna はyamlのroundtripでTazunaの完全なコピーを作る。
// CheckAndFixがバリデーション用のinclude展開で書き戻し対象の構造体を
// 破壊しないために使う。
func deepCopyTazuna(in *v1.Tazuna) (*v1.Tazuna, error) {
	data, err := yaml.Marshal(in)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := &v1.Tazuna{}
	if err := yaml.Unmarshal(data, out); err != nil {
		return nil, errors.WithStack(err)
	}
	return out, nil
}
