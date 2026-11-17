package genesissecret

import (
	"context"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/tfstate"
)

// TFStateProvider は tfstate の値を GenesisSecret に流し込む SecretProvider 実装です。
// tazuna.yaml の spec.providers[] で `type: tfstate` / `tfstate.source: <name>` として
// 宣言し、GenesisSecret の secrets[].items のキーを tfstate のアドレス
// (例: `output.db_password`, `aws_db_instance.main.endpoint`) として解釈します。
//
// 仕様:
//   - secrets[].uri と preferLabel は使用しない (envfile provider と同様)。
//   - string はそのまま、number / bool は文字列化、list / object は JSON 文字列にする。
//   - null はエラーにする (空の Secret 値を黙って作らないため)。
type TFStateProvider struct {
	lookuper tfstate.Lookuper
	source   string
}

var _ SecretProvider = (*TFStateProvider)(nil)

// NewTFStateProvider は source (spec.tfstates[].name) を参照する TFStateProvider を返します。
func NewTFStateProvider(lookuper tfstate.Lookuper, source string) *TFStateProvider {
	return &TFStateProvider{lookuper: lookuper, source: source}
}

// Fetch implements SecretProvider.
func (p *TFStateProvider) Fetch(ctx context.Context, s v1.GenesisSecretGenerate) (map[string]string, error) {
	if p.lookuper == nil {
		return nil, errors.Errorf("tfstate provider for source %q has no resolver", p.source)
	}
	fetched := make(map[string]string, len(s.Items))
	for address := range s.Items {
		v, err := p.lookuper.Lookup(ctx, p.source, address)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		str, err := tfstate.FormatValue(v)
		if err != nil {
			return nil, errors.Wrapf(err, "tfstate source %q: %q cannot be used as a secret value", p.source, address)
		}
		fetched[address] = str
	}
	return mapTo(fetched, s.Items)
}
