// Package oras - auth.go
//
// registry への pull 時に使う credential を解決する。
// 優先順位:
//  1. spec.manifests[].oras.auth の override (username/password どちらかが非空)
//  2. docker config.json 由来の credential (credentials.NewStoreFromDocker)
//  3. anonymous (auth.EmptyCredential)
package oras

import (
	"context"
	"fmt"

	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// CredentialResolver は ORAS pull 時の認証解決を行います。
// プロセス内で共有する auth.Cache を 1 個保持し、
// 複数 manifest の並列 pull 時に token 再取得を抑制する目的で利用します。
type CredentialResolver struct {
	// store は docker config.json 由来の credential store。
	// nil の場合は常に anonymous (auth.EmptyCredential) を返します。
	store credentials.Store
	cache auth.Cache
}

// NewCredentialResolver は docker config.json を読み込む Resolver を返します。
// $DOCKER_CONFIG 環境変数が設定されていれば oras-go がそちらを参照します。
func NewCredentialResolver() (*CredentialResolver, error) {
	st, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, fmt.Errorf("oras auth: load docker credential store: %w", err)
	}
	return &CredentialResolver{
		store: st,
		cache: auth.NewCache(),
	}, nil
}

// NewCredentialResolverWithStore は任意の credentials.Store を注入した Resolver を返します。
// テスト用、あるいは docker config を参照したくないケースで利用します。
// store が nil の場合は常に anonymous を返します。
func NewCredentialResolverWithStore(store credentials.Store) *CredentialResolver {
	return &CredentialResolver{
		store: store,
		cache: auth.NewCache(),
	}
}

// Resolve は registry に対する credential を解決します。
//
// 優先順位:
//  1. override が非 nil かつ Username/Password の少なくとも一方が非空 → override をそのまま返す
//  2. store が非 nil → store.Get の結果を返す (該当エントリが無い場合は anonymous)
//  3. それ以外 → anonymous (auth.EmptyCredential)
//
// override が非 nil でも両フィールドが空の場合は意図しない anonymous 化を防ぐため
// store フォールバックに進みます。
func (r *CredentialResolver) Resolve(ctx context.Context, registry string, override *v1.ORASAuth) (auth.Credential, error) {
	if override != nil && (override.Username != "" || override.Password != "") {
		return auth.Credential{
			Username: override.Username,
			Password: override.Password,
		}, nil
	}
	if r.store == nil {
		return auth.EmptyCredential, nil
	}
	cred, err := r.store.Get(ctx, registry)
	if err != nil {
		return auth.EmptyCredential, fmt.Errorf("oras auth: get credential for %q: %w", registry, err)
	}
	return cred, nil
}

// Cache は manager 単位で共有する auth.Cache を返します。
// Puller がこれを oras-go の auth.Client.Cache に渡すことで、
// 同一 registry への複数 pull で token 再取得を抑制します。
func (r *CredentialResolver) Cache() auth.Cache {
	return r.cache
}
