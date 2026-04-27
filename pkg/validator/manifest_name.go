package validator

import (
	"regexp"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

var manifestNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

const reservedNameMetadata = "_metadata"

// ValidateManifestNames はinclude展開後の全manifest間でnameのバリデーションを行う。
// - Nameが必須
// - 使用可能文字: [a-zA-Z0-9_-]
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

	if !manifestNamePattern.MatchString(manifest.Name) {
		return errors.Errorf("manifest[%d]: name %q contains invalid characters (allowed: a-zA-Z0-9_-)", index, manifest.Name)
	}

	if manifest.Name == reservedNameMetadata {
		return errors.Errorf("manifest[%d]: name %q is reserved", index, manifest.Name)
	}

	return nil
}

// CollectAllManifests はinclude展開後の全manifestをフラットなスライスとして収集する。
// parallel内のchildrenも再帰的に収集する。
func CollectAllManifests(manifests []v1.Manifest) []v1.Manifest {
	var result []v1.Manifest
	for _, m := range manifests {
		result = append(result, m)
		if m.Parallel != nil {
			result = append(result, CollectAllManifests(m.Parallel.Children)...)
		}
	}
	return result
}
