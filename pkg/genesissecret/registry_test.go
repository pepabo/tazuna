package genesissecret_test

import (
	"context"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
)

// stubProvider is a tiny SecretProvider used in registry tests.
type stubProvider struct {
	name string
}

func (s *stubProvider) Fetch(_ context.Context, _ v1.GenesisSecretGenerate) (map[string]string, error) {
	return map[string]string{"name": s.name}, nil
}

func TestProviderRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	r := genesissecret.NewProviderRegistry()
	p := &stubProvider{name: "foo"}
	if err := r.Register("foo", p); err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}

	got, err := r.Get("foo")
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if got != p {
		t.Errorf("Get returned different provider")
	}
}

func TestProviderRegistry_Has(t *testing.T) {
	t.Parallel()
	r := genesissecret.NewProviderRegistry()
	if r.Has("missing") {
		t.Errorf("Has should be false for missing")
	}
	if err := r.Register("foo", &stubProvider{}); err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}
	if !r.Has("foo") {
		t.Errorf("Has should be true for registered")
	}
}

func TestProviderRegistry_RegisterDuplicate(t *testing.T) {
	t.Parallel()
	r := genesissecret.NewProviderRegistry()
	if err := r.Register("foo", &stubProvider{}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := r.Register("foo", &stubProvider{})
	if err == nil {
		t.Fatal("expected duplicate register to fail, got nil")
	}
}

func TestProviderRegistry_RegisterEmptyName(t *testing.T) {
	t.Parallel()
	r := genesissecret.NewProviderRegistry()
	if err := r.Register("", &stubProvider{}); err == nil {
		t.Fatal("expected register with empty name to fail")
	}
}

func TestProviderRegistry_RegisterNilProvider(t *testing.T) {
	t.Parallel()
	r := genesissecret.NewProviderRegistry()
	if err := r.Register("foo", nil); err == nil {
		t.Fatal("expected register with nil provider to fail")
	}
}

func TestProviderRegistry_GetMissing(t *testing.T) {
	t.Parallel()
	r := genesissecret.NewProviderRegistry()
	if _, err := r.Get("missing"); err == nil {
		t.Fatal("expected Get for missing to fail")
	}
}
