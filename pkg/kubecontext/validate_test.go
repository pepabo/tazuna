package kubecontext

import (
	"strings"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
)

func TestMatchContext_OR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		context     string
		patterns    []string
		expectErr   bool
		errContains string
	}{
		{
			name:      "single pattern matches",
			context:   "kind-tazuna-dev",
			patterns:  []string{"^kind-.*$"},
			expectErr: false,
		},
		{
			name:      "first pattern matches",
			context:   "kind-tazuna-dev",
			patterns:  []string{"^kind-.*$", "^prod-.*$"},
			expectErr: false,
		},
		{
			name:      "second pattern matches",
			context:   "prod-cluster-1",
			patterns:  []string{"^kind-.*$", "^prod-.*$"},
			expectErr: false,
		},
		{
			name:        "no pattern matches",
			context:     "staging-cluster",
			patterns:    []string{"^kind-.*$", "^prod-.*$"},
			expectErr:   true,
			errContains: "does not match any of context_matches patterns",
		},
		{
			name:      "anchorless pattern matches whole name",
			context:   "prod",
			patterns:  []string{"prod"},
			expectErr: false,
		},
		{
			name:        "anchorless pattern does not partially match",
			context:     "preprod-cluster",
			patterns:    []string{"prod"},
			expectErr:   true,
			errContains: "does not match any of context_matches patterns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := matchContext(tt.context, tt.patterns, v1.ContextMatchModeOR)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got: %s", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestMatchContext_AND(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		context     string
		patterns    []string
		expectErr   bool
		errContains string
	}{
		{
			name:      "all patterns match",
			context:   "kind-tazuna-dev",
			patterns:  []string{"^kind-.*$", ".*dev$"},
			expectErr: false,
		},
		{
			name:        "one pattern does not match",
			context:     "kind-tazuna-dev",
			patterns:    []string{"^kind-.*$", "^prod-.*$"},
			expectErr:   true,
			errContains: "does not match context_matches patterns",
		},
		{
			name:        "no patterns match",
			context:     "staging-cluster",
			patterns:    []string{"^kind-.*$", "^prod-.*$"},
			expectErr:   true,
			errContains: "does not match context_matches patterns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := matchContext(tt.context, tt.patterns, v1.ContextMatchModeAND)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got: %s", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestMatchContext_DefaultMode(t *testing.T) {
	t.Parallel()
	// 空のmodeはORとして扱われる
	err := matchContext("kind-dev", []string{"^kind-.*$", "^prod-.*$"}, "")
	if err != nil {
		t.Errorf("expected no error with default mode (OR), got: %v", err)
	}
}
