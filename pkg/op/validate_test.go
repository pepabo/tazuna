package op_test

import (
	"testing"

	"github.com/pepabo/tazuna/pkg/op"
)

func TestValidateIdentifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		kind    string
		value   string
		wantErr bool
	}{
		{name: "alnum", kind: "vault", value: "MyVault01", wantErr: false},
		{name: "with hyphen and underscore", kind: "vault", value: "my-vault_01", wantErr: false},
		{name: "with dot", kind: "item", value: "tls.key.v1", wantErr: false},
		{name: "with spaces", kind: "item", value: "Production API Key", wantErr: false},
		{name: "empty", kind: "vault", value: "", wantErr: true},
		{name: "newline", kind: "item", value: "name\nname", wantErr: true},
		{name: "tab", kind: "item", value: "name\tname", wantErr: true},
		{name: "slash", kind: "vault", value: "a/b", wantErr: true},
		{name: "shell metachar", kind: "vault", value: "v$(whoami)", wantErr: true},
		{name: "unicode letters", kind: "vault", value: "サンプル_vault", wantErr: false},
		{name: "unicode katakana", kind: "item", value: "ボールト", wantErr: false},
		{name: "unicode with spaces", kind: "vault", value: "サンプル Vault", wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := op.ValidateIdentifier(tc.kind, tc.value)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateIdentifier(%q, %q) = nil, want error", tc.kind, tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateIdentifier(%q, %q) = %v, want nil", tc.kind, tc.value, err)
			}
		})
	}
}
