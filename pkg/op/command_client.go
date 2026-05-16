package op

import (
	"context"
	"encoding/json"
	"os/exec"

	"github.com/cockroachdb/errors"
)

type CommandClient struct{}

// GetVault implements Client.
func (c *CommandClient) GetVault(ctx context.Context, vaultName string) (Vault, error) {
	if err := ValidateIdentifier("vault", vaultName); err != nil {
		return Vault{}, errors.WithStack(err)
	}
	cmds := NewCommandBuilder().WithVault(
		NewVaultCommandBuilder().
			WithGet(
				NewVaultGetCommandBuilder().WithVault(vaultName),
			),
	).
		WithJSONFormat().
		Build()

	out, err := exec.CommandContext(ctx, cmds[0], cmds[1:]...).Output()
	if err != nil {
		return Vault{}, err
	}

	vault := Vault{}
	if err := json.Unmarshal(out, &vault); err != nil {
		return Vault{}, err
	}

	return vault, nil
}

// GetVaultItem implements Client.
func (c *CommandClient) GetVaultItem(ctx context.Context, vaultName string, itemName string) (Item, error) {
	if err := ValidateIdentifier("vault", vaultName); err != nil {
		return Item{}, errors.WithStack(err)
	}
	if err := ValidateIdentifier("item", itemName); err != nil {
		return Item{}, errors.WithStack(err)
	}
	cmds := NewCommandBuilder().
		WithItem(
			NewItemCommandBuilder().
				WithGet(
					NewItemGetCommandBuilder().
						WithVault(vaultName).WithTitle(itemName),
				),
		).
		WithJSONFormat().
		Build()

	out, err := exec.CommandContext(ctx, cmds[0], cmds[1:]...).Output()
	if err != nil {
		return Item{}, err
	}

	item := Item{}
	if err := json.Unmarshal(out, &item); err != nil {
		return Item{}, err
	}

	return item, nil
}

func (c *CommandClient) ListVaultItems(ctx context.Context, vaultName string) ([]Item, error) {
	if err := ValidateIdentifier("vault", vaultName); err != nil {
		return nil, errors.WithStack(err)
	}
	cmds := NewCommandBuilder().
		WithItem(
			NewItemCommandBuilder().
				WithList(
					NewItemListCommandBuilder().WithVault(vaultName),
				),
		).
		WithJSONFormat().
		Build()

	out, err := exec.CommandContext(ctx, cmds[0], cmds[1:]...).Output()
	if err != nil {
		return nil, err
	}

	var items []Item
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, err
	}

	return items, nil
}

var _ Client = &CommandClient{}
