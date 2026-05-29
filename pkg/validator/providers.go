package validator

import (
	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

// ValidateProviders は tazuna.yaml の spec.providers[] をバリデーションします。
// 重複名・予約名 ("default-op") の使用・type と config フィールドの不整合を検出します。
func ValidateProviders(providers []v1.ProviderConfig) error {
	seen := map[string]bool{}

	for i, pc := range providers {
		if pc.Name == "" {
			return errors.Errorf("providers[%d]: name is required", i)
		}
		if pc.Name == v1.DefaultOnePasswordProviderName {
			return errors.Errorf("providers[%d]: name %q is reserved for the built-in provider",
				i, v1.DefaultOnePasswordProviderName)
		}
		if seen[pc.Name] {
			return errors.Errorf("providers[%d]: duplicate name %q", i, pc.Name)
		}
		seen[pc.Name] = true

		if err := validateProviderConfig(pc); err != nil {
			return errors.Wrapf(err, "providers[%d] (%q)", i, pc.Name)
		}
	}

	return nil
}

// validateProviderConfig は 1 つの ProviderConfig の type と config フィールドの
// 整合性をチェックします。
func validateProviderConfig(pc v1.ProviderConfig) error {
	switch pc.Type {
	case "":
		return errors.New("type is required")
	case v1.ProviderTypeOnePassword:
		// 現状の実装は default-op のみで、ユーザ宣言の onepassword type は
		// runner 側で reject される。validation でも同じ規則を適用する。
		return errors.Errorf("explicit %q type is not supported yet; use the built-in %q",
			v1.ProviderTypeOnePassword, v1.DefaultOnePasswordProviderName)
	case v1.ProviderTypeEnvFile:
		if pc.EnvFile == nil {
			return errors.New("envfile config is required for envfile type")
		}
		if pc.EnvFile.Path == "" {
			return errors.New("envfile.path is required")
		}
		return nil
	default:
		return errors.Errorf("unsupported provider type %q", pc.Type)
	}
}
