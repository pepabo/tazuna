package oras

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2/registry/remote/auth"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// stubStore は credentials.Store を満たすテスト用 fake です。
type stubStore struct {
	creds map[string]auth.Credential
	err   error
}

func (s *stubStore) Get(_ context.Context, server string) (auth.Credential, error) {
	if s.err != nil {
		return auth.EmptyCredential, s.err
	}
	if c, ok := s.creds[server]; ok {
		return c, nil
	}
	return auth.EmptyCredential, nil
}

func (s *stubStore) Put(_ context.Context, _ string, _ auth.Credential) error {
	return nil
}

func (s *stubStore) Delete(_ context.Context, _ string) error {
	return nil
}

func TestCredentialResolver_OverridePriority(t *testing.T) {
	store := &stubStore{
		creds: map[string]auth.Credential{
			"example.com": {Username: "from-store", Password: "store-pass"},
		},
	}
	r := NewCredentialResolverWithStore(store)
	ctx := context.Background()

	t.Run("override wins when username is set", func(t *testing.T) {
		got, err := r.Resolve(ctx, "example.com", &v1.ORASAuth{
			Username: "alice",
			Password: "s3cret",
		})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.Username != "alice" || got.Password != "s3cret" {
			t.Errorf("got %+v, want alice/s3cret", got)
		}
	})

	t.Run("override wins when only password is set", func(t *testing.T) {
		got, err := r.Resolve(ctx, "example.com", &v1.ORASAuth{Password: "only-pass"})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.Username != "" || got.Password != "only-pass" {
			t.Errorf("got %+v, want only password set", got)
		}
	})

	t.Run("empty override falls back to store", func(t *testing.T) {
		got, err := r.Resolve(ctx, "example.com", &v1.ORASAuth{})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.Username != "from-store" || got.Password != "store-pass" {
			t.Errorf("got %+v, want from-store/store-pass", got)
		}
	})

	t.Run("nil override goes to store", func(t *testing.T) {
		got, err := r.Resolve(ctx, "example.com", nil)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.Username != "from-store" {
			t.Errorf("got %+v", got)
		}
	})
}

func TestCredentialResolver_StoreError(t *testing.T) {
	store := &stubStore{err: errors.New("boom")}
	r := NewCredentialResolverWithStore(store)

	got, err := r.Resolve(context.Background(), "example.com", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != auth.EmptyCredential {
		t.Errorf("got %+v, want EmptyCredential", got)
	}
}

func TestCredentialResolver_DockerConfig(t *testing.T) {
	// 最小限の docker config.json を作成し DOCKER_CONFIG を向ける。
	dir := t.TempDir()
	encoded := base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))
	configJSON := `{
  "auths": {
    "localhost:5000": {
      "auth": "` + encoded + `"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(configJSON), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	t.Setenv("DOCKER_CONFIG", dir)

	r, err := NewCredentialResolver()
	if err != nil {
		t.Fatalf("NewCredentialResolver: %v", err)
	}

	t.Run("registered registry resolves credential", func(t *testing.T) {
		got, err := r.Resolve(context.Background(), "localhost:5000", nil)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.Username != "alice" || got.Password != "s3cret" {
			t.Errorf("got %+v, want alice/s3cret", got)
		}
	})

	t.Run("unregistered registry falls back to anonymous", func(t *testing.T) {
		got, err := r.Resolve(context.Background(), "registry.example.com", nil)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got != auth.EmptyCredential {
			t.Errorf("got %+v, want EmptyCredential", got)
		}
	})
}

func TestCredentialResolver_NilStore_AnonymousFallback(t *testing.T) {
	r := NewCredentialResolverWithStore(nil)

	got, err := r.Resolve(context.Background(), "any.example.com", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != auth.EmptyCredential {
		t.Errorf("got %+v, want EmptyCredential", got)
	}
}

func TestCredentialResolver_Cache_NotNil(t *testing.T) {
	r := NewCredentialResolverWithStore(nil)
	if r.Cache() == nil {
		t.Error("Cache() returned nil")
	}
}
