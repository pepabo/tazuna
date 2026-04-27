package op

import (
	"context"
)

type Client interface {
	GetVault(ctx context.Context,
		vaultName string,
	) (Vault, error)
	GetVaultItem(ctx context.Context,
		vaultName string,
		itemName string,
	) (Item, error)
	ListVaultItems(ctx context.Context,
		vaultName string,
	) ([]Item, error)
}
