package op

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCommandClient_GetVaultItem_Memoized(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	c := &CommandClient{
		execCommand: func(ctx context.Context, cmds []string) ([]byte, error) {
			calls.Add(1)
			return []byte(`{"id": "my-item", "fields": [{"id": "f1", "value": "v1"}]}`), nil
		},
	}

	for range 5 {
		item, err := c.GetVaultItem(context.Background(), "my-vault", "my-item")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if item.ID != "my-item" {
			t.Errorf("item.ID = %q, want my-item", item.ID)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("op subprocess invoked %d times, want 1 (memoized)", got)
	}

	// 異なる item はキャッシュミスとして別途取得される
	if _, err := c.GetVaultItem(context.Background(), "my-vault", "other-item"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("op subprocess invoked %d times after second item, want 2", got)
	}
}

func TestCommandClient_GetVaultItem_ConcurrentSingleFetch(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	c := &CommandClient{
		execCommand: func(ctx context.Context, cmds []string) ([]byte, error) {
			calls.Add(1)
			return []byte(`{"id": "my-item"}`), nil
		},
	}

	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.GetVaultItem(context.Background(), "my-vault", "my-item")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("op subprocess invoked %d times under concurrency, want 1", got)
	}
}

func TestCommandClient_GetVaultItem_ErrorNotCached(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	c := &CommandClient{
		execCommand: func(ctx context.Context, cmds []string) ([]byte, error) {
			if calls.Add(1) == 1 {
				return nil, fmt.Errorf("transient failure")
			}
			return []byte(`{"id": "my-item"}`), nil
		},
	}

	if _, err := c.GetVaultItem(context.Background(), "my-vault", "my-item"); err == nil {
		t.Fatal("expected error on first call, got nil")
	}

	// エラーはキャッシュされず、次の呼び出しで再試行される
	item, err := c.GetVaultItem(context.Background(), "my-vault", "my-item")
	if err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if item.ID != "my-item" {
		t.Errorf("item.ID = %q, want my-item", item.ID)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("op subprocess invoked %d times, want 2", got)
	}
}
