package op

import (
	"context"
	"encoding/json"
	"os/exec"
)

type CommandClient struct{}

// GetVault implements Client.
func (c *CommandClient) GetVault(ctx context.Context, vaultName string) (Vault, error) {
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
