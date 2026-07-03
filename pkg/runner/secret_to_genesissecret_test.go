package runner

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pepabo/tazuna/pkg/op"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestSecretToGenesisSecret_DryRun は options 構造体経由の呼び出しで
// namespace / label-selector のフィルタが効き、dry-run 出力が
// w に書き出されることを確認する (cobra 依存除去後の回帰テスト)。
func TestSecretToGenesisSecret_DryRun(t *testing.T) {
	t.Parallel()

	secrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-secret",
				Namespace: "default",
				Labels:    map[string]string{"team": "platform"},
			},
			Data: map[string][]byte{"password": []byte("s3cret")},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-secret",
				Namespace: "default",
				Labels:    map[string]string{"team": "other"},
			},
			Data: map[string][]byte{"k": []byte("v")},
		},
	}
	k8sClient := fake.NewClientBuilder().WithObjects(&secrets[0], &secrets[1]).Build()

	// vault に同名 item が既に存在する状態にして op CLI の実行を回避する
	opClient := op.NewFakeClient()
	opClient.Vaults["my-vault"] = []op.Item{
		{ID: "app-secret", Title: "app-secret"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewTazunaRunner(logger, k8sClient, opClient)

	dumpDir := t.TempDir()
	var out bytes.Buffer
	err := r.SecretToGenesisSecret(context.Background(), SecretToGenesisSecretOptions{
		LabelSelector: "team=platform",
		Vault:         "my-vault",
		Namespace:     "default",
		DryRun:        true,
		DumpDir:       dumpDir,
		Note:          "test",
		OpHost:        "example.1password.com",
	}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// dry-run 出力は w に書かれ、label-selector に合致する Secret のみが対象になる
	if !strings.Contains(out.String(), "app-secret.yaml") {
		t.Errorf("dry-run output missing app-secret: %q", out.String())
	}
	if strings.Contains(out.String(), "other-secret") {
		t.Errorf("label-selector filter did not apply: %q", out.String())
	}

	// dry-run ではファイルを書き出さない
	if _, err := os.Stat(filepath.Join(dumpDir, "app-secret.yaml")); !os.IsNotExist(err) {
		t.Errorf("dry-run should not write files, stat err = %v", err)
	}
}

func TestLabelSelectorStringToMap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{"single", "k=v", map[string]string{"k": "v"}, false},
		{"multi", "k1=v1,k2=v2", map[string]string{"k1": "v1", "k2": "v2"}, false},
		{"invalid no equal", "kv", nil, true},
		{"invalid triple", "k=v=z", nil, true},
		{"empty string", "", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := labelSelectorStringToMap(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%+v)", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("got[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestFilterSecretsWithRegex(t *testing.T) {
	t.Parallel()
	secrets := []corev1.Secret{
		{ObjectMeta: metav1.ObjectMeta{Name: "app-secret"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "db-secret"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "tls-cert"}},
	}

	t.Run("nil regex returns all", func(t *testing.T) {
		t.Parallel()
		got := filterSecretsWithRegex(secrets, nil)
		if len(got) != len(secrets) {
			t.Errorf("len = %d, want %d", len(got), len(secrets))
		}
	})

	t.Run("matches subset", func(t *testing.T) {
		t.Parallel()
		re := regexp.MustCompile(`-secret$`)
		got := filterSecretsWithRegex(secrets, re)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		for _, s := range got {
			if !strings.HasSuffix(s.Name, "-secret") {
				t.Errorf("unexpected name: %q", s.Name)
			}
		}
	})

	t.Run("matches none", func(t *testing.T) {
		t.Parallel()
		re := regexp.MustCompile(`^nomatch`)
		got := filterSecretsWithRegex(secrets, re)
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		got := filterSecretsWithRegex(nil, nil)
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
}

func TestSecretToVaultItem(t *testing.T) {
	t.Parallel()
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret"},
		Data: map[string][]byte{
			"username": []byte("alice"),
			"password": []byte("s3cret"),
		},
	}
	item := secretToVaultItem(secret)

	if item.Title != "my-secret" {
		t.Errorf("Title = %q", item.Title)
	}
	if len(item.Fields) != 2 {
		t.Fatalf("Fields len = %d, want 2", len(item.Fields))
	}
	// データの順序は map なので保証されないため、map にして検証
	got := map[string]string{}
	for _, f := range item.Fields {
		if f.Type != op.ItemFieldTypeString {
			t.Errorf("Type = %q", f.Type)
		}
		got[f.Label] = f.Value
	}
	if got["username"] != "alice" || got["password"] != "s3cret" {
		t.Errorf("fields = %+v", got)
	}
}

func TestSecretsToVaultItems(t *testing.T) {
	t.Parallel()
	secrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "s1"},
			Data:       map[string][]byte{"k": []byte("v")},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "s2"},
			Data:       map[string][]byte{"a": []byte("b")},
		},
	}
	vault := &op.Vault{Name: "my-vault"}

	items := secretsToVaultItems(secrets, vault, "test note")

	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	for i, item := range items {
		if item.Vault.Name != "my-vault" {
			t.Errorf("items[%d].Vault.Name = %q", i, item.Vault.Name)
		}
		// notes は最後のフィールドとして追加される
		last := item.Fields[len(item.Fields)-1]
		if last.ID != op.ItemIDNotesPlain {
			t.Errorf("items[%d] last field ID = %q, want %q", i, last.ID, op.ItemIDNotesPlain)
		}
		if last.Purpose != op.ItemPurposeNotes {
			t.Errorf("items[%d] last field Purpose = %q", i, last.Purpose)
		}
		if !strings.Contains(last.Value, "test note") {
			t.Errorf("items[%d] notes does not contain user note: %q", i, last.Value)
		}
		if !strings.Contains(last.Value, "Tazuna") {
			t.Errorf("items[%d] notes missing tazuna marker: %q", i, last.Value)
		}
	}
}

func TestItemCreateCommandsFromItems(t *testing.T) {
	t.Parallel()
	items := []op.Item{
		{Title: "item1", Fields: []op.ItemField{{Label: "k", Value: "v"}}},
		{Title: "item2", Fields: []op.ItemField{{Label: "a", Value: "b"}}},
	}

	t.Run("dry-run flag added", func(t *testing.T) {
		t.Parallel()
		cmds, err := itemCreateCommandsFromItems(context.Background(), items, "my-vault", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cmds) != 2 {
			t.Fatalf("cmds len = %d, want 2", len(cmds))
		}
		for i, cmd := range cmds {
			args := strings.Join(cmd.Args, " ")
			if !strings.Contains(args, "--dry-run") {
				t.Errorf("cmds[%d] missing --dry-run: %s", i, args)
			}
			if !strings.Contains(args, "my-vault") {
				t.Errorf("cmds[%d] missing vault name: %s", i, args)
			}
			if cmd.Stdin == nil {
				t.Errorf("cmds[%d] Stdin is nil", i)
			}
		}
	})

	t.Run("non-dry-run no flag", func(t *testing.T) {
		t.Parallel()
		cmds, err := itemCreateCommandsFromItems(context.Background(), items, "v", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for i, cmd := range cmds {
			args := strings.Join(cmd.Args, " ")
			if strings.Contains(args, "--dry-run") {
				t.Errorf("cmds[%d] unexpectedly contains --dry-run: %s", i, args)
			}
		}
	})

	t.Run("title in command", func(t *testing.T) {
		t.Parallel()
		cmds, _ := itemCreateCommandsFromItems(context.Background(), items, "v", false)
		args0 := strings.Join(cmds[0].Args, " ")
		args1 := strings.Join(cmds[1].Args, " ")
		if !strings.Contains(args0, "item1") {
			t.Errorf("cmds[0] missing title: %s", args0)
		}
		if !strings.Contains(args1, "item2") {
			t.Errorf("cmds[1] missing title: %s", args1)
		}
	})
}
