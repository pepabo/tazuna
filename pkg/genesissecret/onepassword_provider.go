package genesissecret

import (
	"context"
	"fmt"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/op"
)

type OnePasswordProvider struct {
	client op.Client
}

// Fetch implements SecretProvider.
func (o *OnePasswordProvider) Fetch(ctx context.Context, s v1.GenesisSecretGenerate) (map[string]string, error) {
	vault, itemName, err := ParseOnePasswordURI(s.URI)
	if err != nil {
		return nil, err
	}

	item, err := o.client.GetVaultItem(ctx, vault, itemName)
	if err != nil {
		return nil, err
	}

	ret := map[string]string{}
	for _, f := range item.Fields {
		if s.PreferLabel {
			ret[f.Label] = f.Value
		} else {
			ret[f.ID] = f.Value
		}
	}

	return mapTo(ret, s.Items)
}

var _ SecretProvider = &OnePasswordProvider{}

func mapTo(fetched map[string]string, items map[string]v1.GenesisSecretGenerateItem) (map[string]string, error) {
	ret := map[string]string{}

	for k, i := range items {
		if v, ok := fetched[k]; ok {
			ret[i.MapTo] = v
		} else {
			return nil, fmt.Errorf("key %s not found in fetched data", k)
		}
	}
	return ret, nil
}

func NewOnePasswordProvider(client op.Client) *OnePasswordProvider {
	return &OnePasswordProvider{
		client: client,
	}
}
