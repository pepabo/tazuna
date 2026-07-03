package validator

import (
	"regexp"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

// manifestNamePattern はDNS-1123 subdomain相当の命名規則。
// manifest名は state ConfigMap 名 (tazuna-state-<name>) にそのまま使われ、
// state key のエンコード ('/' ↔ '__' 置換) とも衝突してはならないため、
// 小文字英数と '-' のみを許可する ('_' や大文字は不可)。
var manifestNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// maxManifestNameLength はmanifest名の最大長。
// ConfigMap名の上限 (253) から prefix "tazuna-state-" (13文字) を引いた値。
const maxManifestNameLength = 240

const reservedNameMetadata = "_metadata"

// ValidateManifestNames はinclude展開後の全manifest間でnameのバリデーションを行う。
// - Nameが必須
// - 使用可能文字: 小文字英数と '-' (DNS-1123 相当、先頭末尾は英数)
// - 最大長 240 文字
// - ユニーク制約
// - 予約語 _metadata の禁止
func ValidateManifestNames(manifests []v1.Manifest) error {
	names := make(map[string]int) // name -> 出現回数
	var errs []error

	for i := range manifests {
		if err := validateSingleManifestName(&manifests[i], i); err != nil {
			errs = append(errs, err)
			continue
		}
		names[manifests[i].Name]++
	}

	// ユニーク制約チェック
	for name, count := range names {
		if count > 1 {
			errs = append(errs, errors.Errorf("manifest name %q is duplicated (%d times)", name, count))
		}
	}

	return errors.Join(errs...)
}

func validateSingleManifestName(manifest *v1.Manifest, index int) error {
	if manifest.Name == "" {
		return errors.Errorf("manifest[%d]: name is required", index)
	}

	if manifest.Name == reservedNameMetadata {
		return errors.Errorf("manifest[%d]: name %q is reserved", index, manifest.Name)
	}

	if len(manifest.Name) > maxManifestNameLength {
		return errors.Errorf("manifest[%d]: name %q is too long (%d chars, max %d)", index, manifest.Name, len(manifest.Name), maxManifestNameLength)
	}

	if !manifestNamePattern.MatchString(manifest.Name) {
		return errors.Errorf("manifest[%d]: name %q contains invalid characters (allowed: lowercase a-z0-9 and '-', must start and end with an alphanumeric)", index, manifest.Name)
	}

	return nil
}
