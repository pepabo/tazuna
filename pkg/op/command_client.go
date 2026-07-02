package op

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"

	"github.com/cockroachdb/errors"
)

type CommandClient struct {
	// execCommand はテストから op CLI の実行を差し替えるためのフック。
	// nil の場合は実際に op CLI を実行する。
	execCommand func(ctx context.Context, cmds []string) ([]byte, error)

	// itemMu / itemCache は GetVaultItem 結果の vault/item 単位の memoize。
	// helmfile vars で同じ 1Password item の複数 field を参照すると、var の数だけ
	// op サブプロセス (1 回 0.5〜2 秒) が起動してしまうのを防ぐ。
	// 並列 apply から呼ばれるため mutex で保護する。
	itemMu    sync.Mutex
	itemCache map[string]*itemCacheEntry
}

// itemCacheEntry は同一 vault/item への同時 GetVaultItem を 1 回の op 実行に
// まとめるためのエントリ (singleflight 相当)。
type itemCacheEntry struct {
	once sync.Once
	item Item
	err  error
}

// run は op CLI を実行して stdout を返す。
func (c *CommandClient) run(ctx context.Context, cmds []string) ([]byte, error) {
	if c.execCommand != nil {
		return c.execCommand(ctx, cmds)
	}
	return exec.CommandContext(ctx, cmds[0], cmds[1:]...).Output()
}

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

	out, err := c.run(ctx, cmds)
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
// 同一 vault/item への呼び出しは memoize され、op サブプロセスの起動は
// クライアントの生存期間中 1 回に抑えられる。エラーはキャッシュされず、
// 次の呼び出しで再試行される。
func (c *CommandClient) GetVaultItem(ctx context.Context, vaultName string, itemName string) (Item, error) {
	if err := ValidateIdentifier("vault", vaultName); err != nil {
		return Item{}, errors.WithStack(err)
	}
	if err := ValidateIdentifier("item", itemName); err != nil {
		return Item{}, errors.WithStack(err)
	}

	key := vaultName + "/" + itemName

	c.itemMu.Lock()
	if c.itemCache == nil {
		c.itemCache = map[string]*itemCacheEntry{}
	}
	entry, ok := c.itemCache[key]
	if !ok {
		entry = &itemCacheEntry{}
		c.itemCache[key] = entry
	}
	c.itemMu.Unlock()

	entry.once.Do(func() {
		entry.item, entry.err = c.fetchVaultItem(ctx, vaultName, itemName)
	})

	if entry.err != nil {
		// 失敗をキャッシュに残すと transient なエラーでも以降の参照が全滅する
		// ため、エントリを取り除いて次の呼び出しで再試行できるようにする。
		c.itemMu.Lock()
		if c.itemCache[key] == entry {
			delete(c.itemCache, key)
		}
		c.itemMu.Unlock()
		return Item{}, entry.err
	}

	return entry.item, nil
}

// fetchVaultItem は op CLI で item を取得する (キャッシュを介さない実体)。
func (c *CommandClient) fetchVaultItem(ctx context.Context, vaultName string, itemName string) (Item, error) {
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

	out, err := c.run(ctx, cmds)
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

	out, err := c.run(ctx, cmds)
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
