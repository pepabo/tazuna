package validator

import (
	"sort"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/tfstate"
)

// ValidateTFStates は spec.tfstates[] をバリデーションします。
// name の必須・重複、backend type が対応済みであること、type に対応する設定
// (s3.bucket / s3.key, local.path 等) が揃っていることを検査します。
// backend へのアクセス (認証情報のロードを含む) は行いません。
func ValidateTFStates(sources []v1.TFStateSource) error {
	factories := tfstate.DefaultFactories()
	seen := map[string]bool{}
	for i, s := range sources {
		if s.Name == "" {
			return errors.Errorf("tfstates[%d]: name is required", i)
		}
		if seen[s.Name] {
			return errors.Errorf("tfstates[%d]: duplicate name %q", i, s.Name)
		}
		seen[s.Name] = true

		if err := validateTFStateBackendExclusive(s.Backend); err != nil {
			return errors.Wrapf(err, "tfstates[%d] (%q)", i, s.Name)
		}
		if _, err := factories.New(s.Backend, ""); err != nil {
			return errors.Wrapf(err, "tfstates[%d] (%q)", i, s.Name)
		}
	}
	return nil
}

// validateTFStateBackendExclusive は type と異なる backend 設定が書かれていないかを検査します。
func validateTFStateBackendExclusive(b v1.TFStateBackend) error {
	set := map[v1.TFStateBackendType]bool{
		v1.TFStateBackendTypeS3:    b.S3 != nil,
		v1.TFStateBackendTypeLocal: b.Local != nil,
	}
	var extra []string
	for t, ok := range set {
		if ok && t != b.Type {
			extra = append(extra, string(t))
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return errors.Errorf("backend.type is %q but config for %v is also set", b.Type, extra)
	}
	return nil
}

// ValidateTFStateRef は helmfile var 等で使う TFStateRef の形式を検査します。
func ValidateTFStateRef(ref *v1.TFStateRef) error {
	if ref == nil {
		return errors.New("tfstate field is required")
	}
	if ref.Source == "" {
		return errors.New("tfstate.source is required")
	}
	if ref.Address == "" {
		return errors.New("tfstate.address is required")
	}
	if _, err := tfstate.ParseAddress(ref.Address); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ValidateTFStateReferences は helmfile vars (`from: tfstate`) と tfstate 型 provider が
// 参照する source が spec.tfstates[] に宣言されていることを検査します。
func ValidateTFStateReferences(spec *v1.TazunaSpec) error {
	declared := map[string]bool{}
	for _, s := range spec.TFStates {
		declared[s.Name] = true
	}

	for i, m := range spec.Manifests {
		if m.Helmfile == nil {
			continue
		}
		names := make([]string, 0, len(m.Helmfile.Vars))
		for n := range m.Helmfile.Vars {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			v := m.Helmfile.Vars[n]
			if v.From != v1.HelmFileVarFromTFState || v.TFState == nil {
				continue
			}
			if !declared[v.TFState.Source] {
				return errors.Errorf("manifest[%d] (%q): helmfile var '%s' refers to tfstate source %q which is not declared in spec.tfstates",
					i, m.Name, n, v.TFState.Source)
			}
		}
	}

	for i, p := range spec.Providers {
		if p.Type != v1.ProviderTypeTFState || p.TFState == nil {
			continue
		}
		if !declared[p.TFState.Source] {
			return errors.Errorf("providers[%d] (%q): tfstate source %q is not declared in spec.tfstates",
				i, p.Name, p.TFState.Source)
		}
	}
	return nil
}
