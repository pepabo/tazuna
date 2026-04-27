package op

import "context"

type FakeClient struct {
	Vaults map[string][]Item
}

// ListVaultItems implements Client.
func (f *FakeClient) ListVaultItems(ctx context.Context, vaultName string) ([]Item, error) {
	if items, ok := f.Vaults[vaultName]; ok {
		return items, nil
	}
	return nil, nil
}

// GetVault implements Client.
func (f *FakeClient) GetVault(ctx context.Context, vaultName string) (Vault, error) {
	if _, ok := f.Vaults[vaultName]; ok {
		return Vault{Name: vaultName}, nil
	}
	return Vault{}, nil
}

// GetVaultItem implements Client.
func (f *FakeClient) GetVaultItem(ctx context.Context, vaultName string, itemName string) (Item, error) {
	if items, ok := f.Vaults[vaultName]; ok {
		for _, item := range items {
			if item.ID == itemName {
				return item, nil
			}
		}
	}

	return Item{}, nil
}

var _ Client = &FakeClient{}

func NewFakeClient() *FakeClient {
	return &FakeClient{
		Vaults: map[string][]Item{},
	}
}
