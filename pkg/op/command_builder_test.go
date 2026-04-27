package op_test

import (
	"testing"

	"github.com/pepabo/tazuna/pkg/op"
	"github.com/stretchr/testify/assert"
)

func TestCommandBuild(t *testing.T) {
	t.Parallel()
	t.Run("item_get", itemGetTest)
	t.Run("item create", itemCreateTest)
	t.Run("item_list", itemListTest)
	t.Run("vault_list", vaultListTest)
	t.Run("vault_get", vaultGetTest)
}

func itemListTest(t *testing.T) {
	cmd := op.NewCommandBuilder().
		WithItem(op.NewItemCommandBuilder().WithList(
			op.NewItemListCommandBuilder().WithVault("test"))).Build()

	expected := []string{"op", "item", "list", "--vault", "test"}

	assert.Equal(t, expected, cmd)
}

func itemGetTest(t *testing.T) {
	cmd := op.NewCommandBuilder().
		WithItem(op.NewItemCommandBuilder().WithGet(
			op.NewItemGetCommandBuilder().WithVault("test").WithTitle("test"))).Build()

	expected := []string{"op", "item", "get", "--vault", "test", "test"}

	assert.Equal(t, expected, cmd)
}

func itemCreateTest(t *testing.T) {
	cmd := op.NewCommandBuilder().
		WithItem(op.NewItemCommandBuilder().WithCreate(
			op.NewItemCreateCommandBuilder().WithTitle("test").WithStdin(true).WithDryRun(true))).Build()

	expected := []string{"op", "item", "create", "--title", "test", "--dry-run", "-"}

	assert.Equal(t, expected, cmd)
}

func vaultListTest(t *testing.T) {
	cmd := op.NewCommandBuilder().
		WithVault(op.NewVaultCommandBuilder().WithList(
			op.NewVaultListCommandBuilder())).Build()

	expected := []string{"op", "vault", "list"}

	assert.Equal(t, expected, cmd)
}

func vaultGetTest(t *testing.T) {
	cmd := op.NewCommandBuilder().
		WithVault(op.NewVaultCommandBuilder().WithGet(
			op.NewVaultGetCommandBuilder().WithVault("test"))).Build()

	expected := []string{"op", "vault", "get", "test"}

	assert.Equal(t, expected, cmd)
}
