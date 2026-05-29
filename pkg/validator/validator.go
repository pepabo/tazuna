package validator

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

// ValidateTazuna は Tazuna 構造体全体をバリデーションします
func ValidateTazuna(tazuna *v1.Tazuna) error {
	if tazuna == nil {
		return errors.New("tazuna is nil")
	}

	if err := ValidateTazunaTypeMeta(tazuna); err != nil {
		return errors.WithStack(err)
	}

	if err := ValidateTazunaSpec(&tazuna.Spec, ""); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// ValidateTazunaWithBasePath は Tazuna 構造体全体をバリデーションします（ベースパス付き）
func ValidateTazunaWithBasePath(tazuna *v1.Tazuna, basePath string) error {
	if tazuna == nil {
		return errors.New("tazuna is nil")
	}

	if err := ValidateTazunaTypeMeta(tazuna); err != nil {
		return errors.WithStack(err)
	}

	if err := ValidateTazunaSpec(&tazuna.Spec, basePath); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// ValidateTazunaTypeMeta は Tazuna の apiVersion / kind を検証します。
// 後方互換のため未設定 (空文字) は許容しますが、値が設定されている場合は
// v1.TazunaAPIVersion / v1.TazunaKind と完全一致している必要があります。
func ValidateTazunaTypeMeta(tazuna *v1.Tazuna) error {
	if tazuna.APIVersion != "" && tazuna.APIVersion != v1.TazunaAPIVersion {
		return errors.Errorf("apiVersion must be %q, got %q", v1.TazunaAPIVersion, tazuna.APIVersion)
	}
	if tazuna.Kind != "" && tazuna.Kind != v1.TazunaKind {
		return errors.Errorf("kind must be %q, got %q", v1.TazunaKind, tazuna.Kind)
	}
	return nil
}

// ValidateTazunaSpec は TazunaSpec をバリデーションします
func ValidateTazunaSpec(spec *v1.TazunaSpec, basePath string) error {
	if spec == nil {
		return errors.New("tazuna spec is nil")
	}

	for i, pattern := range spec.ContextMatches {
		if _, err := regexp.Compile(pattern); err != nil {
			return errors.Errorf("context_matches[%d] is not a valid regex: %s", i, err)
		}
	}

	if spec.ContextMatchMode != "" &&
		spec.ContextMatchMode != v1.ContextMatchModeOR &&
		spec.ContextMatchMode != v1.ContextMatchModeAND {
		return errors.Errorf("context_match_mode must be 'or' or 'and', got: %s", spec.ContextMatchMode)
	}

	for i, manifest := range spec.Manifests {
		if err := ValidateManifest(&manifest, basePath); err != nil {
			return errors.Wrapf(err, "validation failed for manifest[%d]", i)
		}
	}

	if err := ValidateProviders(spec.Providers); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// ValidateManifest は Manifest をバリデーションします
func ValidateManifest(manifest *v1.Manifest, basePath string) error {
	if manifest == nil {
		return errors.New("manifest is nil")
	}

	// includesが指定されている場合の特別なバリデーション
	if len(manifest.Includes) > 0 {
		return ValidateManifestWithIncludes(manifest, basePath)
	}

	// パスの必須チェック
	if manifest.Path == "" {
		return errors.New("manifest path is required")
	}

	// タイプの必須チェック
	if manifest.Type == "" {
		return errors.New("manifest type is required")
	}

	// タイプ別のバリデーション
	switch manifest.Type {
	case v1.ManifestTypeHelmfile:
		if err := ValidateManifestHelmfile(manifest.Helmfile); err != nil {
			return errors.Wrapf(err, "helmfile validation failed for manifest %s", manifest.Path)
		}
	case v1.ManifestTypeKustomize:
		// Kustomize特有のバリデーションがあればここに追加
	case v1.ManifestTypeGenesisSecret:
		// GenesisSecret特有のバリデーションがあればここに追加
	case v1.ManifestTypeORAS:
		if err := ValidateManifestORAS(manifest.ORAS); err != nil {
			return errors.Wrapf(err, "oras validation failed for manifest %s", manifest.Path)
		}
	default:
		return errors.Errorf("unsupported manifest type: %s", manifest.Type)
	}

	return nil
}

// ValidateManifestHelmfile は ManifestHelmfile をバリデーションします
func ValidateManifestHelmfile(helmfile *v1.ManifestHelmfile) error {
	if helmfile == nil {
		return nil // helmfileはoptional
	}

	for varName, helmVar := range helmfile.Vars {
		if err := ValidateHelmFileVar(&helmVar, varName); err != nil {
			return errors.Wrapf(err, "helmfile var '%s' validation failed", varName)
		}
	}

	return nil
}

// ValidateHelmFileVar は HelmFileVar をバリデーションします
func ValidateHelmFileVar(helmVar *v1.HelmFileVar, varName string) error {
	if helmVar == nil {
		return errors.Errorf("helmfile var '%s' is nil", varName)
	}

	// From フィールドの必須チェック
	if helmVar.From == "" {
		return errors.Errorf("helmfile var '%s' has no From field, supported From is 'env/static/op'", varName)
	}

	switch helmVar.From {
	case v1.HelmFileVarFromEnv:
		if helmVar.Env == nil {
			return errors.Errorf("helmfile var '%s' has From env but no env field", varName)
		}
	case v1.HelmFileVarFromStatic:
		if err := ValidateStaticVar(helmVar, varName); err != nil {
			return errors.WithStack(err)
		}
	case v1.HelmFileVarFromOp:
		if helmVar.Op == nil {
			return errors.Errorf("helmfile var '%s' has From op but no op field", varName)
		}
		if err := ValidateOnePasswordVaultSelector(helmVar.Op, varName); err != nil {
			return errors.WithStack(err)
		}
	default:
		return errors.Errorf("helmfile var '%s' has unsupported From field: %s, supported From is 'env/static/op'", varName, helmVar.From)
	}

	return nil
}

// ValidateStaticVar は static 系のフィールドのバリデーションを行います
func ValidateStaticVar(helmVar *v1.HelmFileVar, varName string) error {
	// static, staticSlice, staticMapのいずれか1つのみが設定されているかチェック
	count := 0
	if helmVar.Static != nil {
		count++
	}
	if helmVar.StaticSlice != nil {
		count++
	}
	if helmVar.StaticMap != nil {
		count++
	}

	if count == 0 {
		return errors.Errorf("helmfile var '%s' has From static but no static/staticSlice/staticMap field", varName)
	}
	if count > 1 {
		return errors.Errorf("helmfile var '%s' has From static but multiple static fields are set (static: %v, staticSlice: %v, staticMap: %v)",
			varName, helmVar.Static != nil, helmVar.StaticSlice != nil, helmVar.StaticMap != nil)
	}

	return nil
}

// ValidateOnePasswordVaultSelector は OnePasswordVaultSelector をバリデーションします
func ValidateOnePasswordVaultSelector(op *v1.OnePasswordVaultSelector, varName string) error {
	if op == nil {
		return errors.Errorf("helmfile var '%s' has From op but no op field", varName)
	}

	if op.Key == "" {
		return errors.Errorf("helmfile var '%s' has From op but op.key field is empty", varName)
	}
	if op.Vault == "" {
		return errors.Errorf("helmfile var '%s' has From op but op.vault field is empty", varName)
	}
	if op.Item == "" {
		return errors.Errorf("helmfile var '%s' has From op but op.item field is empty", varName)
	}
	if op.Field == "" {
		return errors.Errorf("helmfile var '%s' has From op but op.field field is empty", varName)
	}

	// keyの値チェック
	if op.Key != v1.HelmFileVarOpKeyID && op.Key != v1.HelmFileVarOpKeyLabel {
		return errors.Errorf("helmfile var '%s' has From op but op.key field has invalid value: %s (supported: %s, %s)",
			varName, op.Key, v1.HelmFileVarOpKeyID, v1.HelmFileVarOpKeyLabel)
	}

	return nil
}

// ValidateManifestORAS は ManifestORAS をバリデーションします
func ValidateManifestORAS(oras *v1.ManifestORAS) error {
	if oras == nil {
		return errors.New("oras spec is required for oras manifest")
	}
	if oras.Reference == "" {
		return errors.New("oras.reference is required")
	}
	switch oras.Delegate.Type {
	case v1.ORASDelegateTypeHelmfile:
		if err := ValidateManifestHelmfile(oras.Delegate.Helmfile); err != nil {
			return errors.Wrap(err, "oras.delegate.helmfile validation failed")
		}
	case v1.ORASDelegateTypeKustomize:
		// kustomize固有の追加バリデーションは現状なし
	default:
		return errors.Errorf("oras.delegate.type must be 'helmfile' or 'kustomize', got: %q", oras.Delegate.Type)
	}
	return nil
}

// ValidateManifestWithIncludes は includes を含む Manifest のバリデーションを行います
func ValidateManifestWithIncludes(manifest *v1.Manifest, basePath string) error {
	// includesが空でないことを確認
	if len(manifest.Includes) == 0 {
		return errors.New("includes is specified but empty")
	}

	// includeファイルのパスバリデーション
	for i, include := range manifest.Includes {
		if include.Path == "" {
			return errors.Errorf("include[%d].path is required", i)
		}

		// ベースパスが指定されている場合、includeファイルの存在確認
		if basePath != "" {
			includePath := filepath.Join(basePath, include.Path)
			if _, err := os.Stat(includePath); os.IsNotExist(err) {
				return errors.Errorf("include file not found: %s", includePath)
			}
		}
	}

	// includesが指定されている場合、他のマニフェスト固有フィールドは無視されることを警告
	// ただし、エラーにはしない（プランでは無視すると記載されている）

	return nil
}
