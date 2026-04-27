package genesissecret_test

import (
	"context"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
	"github.com/pepabo/tazuna/pkg/op"
)

func newFakeWithItem(vault, itemID string, fields []op.ItemField) *op.FakeClient {
	c := op.NewFakeClient()
	c.Vaults[vault] = []op.Item{
		{
			ID:     itemID,
			Title:  itemID,
			Fields: fields,
		},
	}
	return c
}

func TestOnePasswordProvider_Fetch_PreferID(t *testing.T) {
	t.Parallel()
	fake := newFakeWithItem("my-vault", "my-item", []op.ItemField{
		{ID: "id-user", Label: "username", Value: "alice"},
		{ID: "id-pass", Label: "password", Value: "s3cret"},
	})
	p := genesissecret.NewOnePasswordProvider(fake)

	got, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		PreferLabel: false,
		URI:         "op://example.1password.com/my-vault/my-item",
		Items: map[string]v1.GenesisSecretGenerateItem{
			"id-user": {MapTo: "USER"},
			"id-pass": {MapTo: "PASS"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["USER"] != "alice" {
		t.Errorf("USER = %q, want alice", got["USER"])
	}
	if got["PASS"] != "s3cret" {
		t.Errorf("PASS = %q, want s3cret", got["PASS"])
	}
}

func TestOnePasswordProvider_Fetch_PreferLabel(t *testing.T) {
	t.Parallel()
	fake := newFakeWithItem("v", "item", []op.ItemField{
		{ID: "id1", Label: "username", Value: "alice"},
		{ID: "id2", Label: "password", Value: "p4ss"},
	})
	p := genesissecret.NewOnePasswordProvider(fake)

	got, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		PreferLabel: true,
		URI:         "op://host/v/item",
		Items: map[string]v1.GenesisSecretGenerateItem{
			"username": {MapTo: "u"},
			"password": {MapTo: "p"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["u"] != "alice" || got["p"] != "p4ss" {
		t.Errorf("got = %+v", got)
	}
}

func TestOnePasswordProvider_Fetch_MissingKey(t *testing.T) {
	t.Parallel()
	fake := newFakeWithItem("v", "item", []op.ItemField{
		{ID: "id1", Label: "username", Value: "alice"},
	})
	p := genesissecret.NewOnePasswordProvider(fake)

	_, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		PreferLabel: true,
		URI:         "op://host/v/item",
		Items: map[string]v1.GenesisSecretGenerateItem{
			"nonexistent": {MapTo: "x"},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestOnePasswordProvider_Fetch_EmptyItems(t *testing.T) {
	t.Parallel()
	fake := newFakeWithItem("v", "item", []op.ItemField{
		{ID: "id1", Label: "k", Value: "v"},
	})
	p := genesissecret.NewOnePasswordProvider(fake)

	got, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		PreferLabel: true,
		URI:         "op://host/v/item",
		Items:       map[string]v1.GenesisSecretGenerateItem{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}
