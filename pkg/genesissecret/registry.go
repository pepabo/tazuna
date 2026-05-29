package genesissecret

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ProviderRegistry は name -> SecretProvider のマップです。
// runner.setupManagers から差し込まれ、GenesisSecret manager が manifest の
// .spec.provider の値を name として SecretProvider を解決するために使われます。
//
// Register は同一 name の重複登録をエラーにします。Get は登録されていない
// name に対してエラーを返します。両者ともスレッドセーフです。
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]SecretProvider
}

// NewProviderRegistry creates an empty provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: map[string]SecretProvider{},
	}
}

// Register adds a SecretProvider under the given name. It returns an error if
// the name is empty, the provider is nil, or a provider with the same name has
// already been registered.
func (r *ProviderRegistry) Register(name string, p SecretProvider) error {
	if name == "" {
		return fmt.Errorf("provider name must not be empty")
	}
	if p == nil {
		return fmt.Errorf("provider %q must not be nil", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %q is already registered", name)
	}
	r.providers[name] = p
	return nil
}

// Get returns the SecretProvider registered under the given name.
// It returns an error including the list of available providers if not found.
func (r *ProviderRegistry) Get(name string) (SecretProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found (registered: %s)", name, r.namesLocked())
	}
	return p, nil
}

// Has reports whether a SecretProvider is registered under the given name.
func (r *ProviderRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.providers[name]
	return ok
}

// namesLocked は r.mu を読みロック中である前提で、登録済み provider 名のソート
// 済みリストをカンマ区切り文字列で返す。エラーメッセージのみで使う想定。
func (r *ProviderRegistry) namesLocked() string {
	if len(r.providers) == 0 {
		return "<none>"
	}
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
