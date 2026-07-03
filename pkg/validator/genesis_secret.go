package validator

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
	"sigs.k8s.io/yaml"
)

// ValidateManifestGenesisSecret は genesissecret manifest が指す spec ファイルを読み、
// 内容をバリデーションします。ファイルが存在しない・読めない・パースできない場合は
// ここではエラーにしません（適用時に別途エラーとして報告されるため）。
func ValidateManifestGenesisSecret(manifest *v1.Manifest, basePath string) error {
	path := manifest.Path
	if basePath != "" {
		path = filepath.Join(basePath, manifest.Path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	gs := v1.GenesisSecret{}
	if err := yaml.Unmarshal(data, &gs); err != nil {
		return nil
	}

	return errors.WithStack(ValidateGenesisSecretSpec(&gs.Spec))
}

// ValidateGenesisSecretSpec は GenesisSecretSpec をバリデーションします。
// 組み込みの 1Password provider（provider 未指定 or "default-op"）を使う場合、
// secrets[].uri が `op://<host>/<vault>/<item>` 形式であることを検証します。
func ValidateGenesisSecretSpec(spec *v1.GenesisSecretSpec) error {
	if spec == nil {
		return errors.New("genesissecret spec is nil")
	}

	usesOnePassword := spec.Provider == "" || spec.Provider == v1.DefaultOnePasswordProviderName
	if !usesOnePassword {
		return nil
	}

	for i, s := range spec.Secrets {
		if _, _, err := genesissecret.ParseOnePasswordURI(s.URI); err != nil {
			return errors.Wrapf(err, "secrets[%d].uri validation failed", i)
		}
	}

	return nil
}
