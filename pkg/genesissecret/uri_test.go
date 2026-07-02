package genesissecret_test

import (
	"testing"

	"github.com/pepabo/tazuna/pkg/genesissecret"
)

func TestParseOnePasswordURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		uri       string
		wantVault string
		wantItem  string
		wantErr   bool
	}{
		{
			name:      "valid uri",
			uri:       "op://example.1password.com/Platform/aws-credentials",
			wantVault: "Platform",
			wantItem:  "aws-credentials",
		},
		{
			name:    "missing host (op CLI style op://<vault>/<item>)",
			uri:     "op://Platform/aws-credentials",
			wantErr: true,
		},
		{
			name:    "missing item",
			uri:     "op://example.1password.com/Platform",
			wantErr: true,
		},
		{
			name:    "extra path segment",
			uri:     "op://example.1password.com/Platform/item/field",
			wantErr: true,
		},
		{
			name:    "empty vault",
			uri:     "op://example.1password.com//item",
			wantErr: true,
		},
		{
			name:    "wrong scheme",
			uri:     "https://example.1password.com/Platform/item",
			wantErr: true,
		},
		{
			name:    "empty uri",
			uri:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vault, item, err := genesissecret.ParseOnePasswordURI(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for uri %q, got vault=%q item=%q", tt.uri, vault, item)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if vault != tt.wantVault || item != tt.wantItem {
				t.Errorf("got vault=%q item=%q, want vault=%q item=%q", vault, item, tt.wantVault, tt.wantItem)
			}
		})
	}
}
