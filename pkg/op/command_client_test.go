package op

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWrapOpCLIError_IncludesStderr(t *testing.T) {
	t.Parallel()

	// 本物の *exec.ExitError (Stderr 付き) を得るため、stderr に書いて失敗する
	// コマンドを実行する
	_, err := exec.CommandContext(context.Background(), "sh", "-c", "echo '[ERROR] vault not found' >&2; exit 1").Output()
	if err == nil {
		t.Fatal("expected command to fail")
	}

	wrapped := wrapOpCLIError(err)
	if !strings.Contains(wrapped.Error(), "vault not found") {
		t.Errorf("wrapped error does not include stderr: %v", wrapped)
	}
}

func TestWrapOpCLIError_NonExitError(t *testing.T) {
	t.Parallel()

	wrapped := wrapOpCLIError(fmt.Errorf("some error"))
	if !strings.Contains(wrapped.Error(), "op CLI failed") {
		t.Errorf("unexpected wrapped error: %v", wrapped)
	}
}

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
