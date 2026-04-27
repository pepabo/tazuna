package prompt_test

import (
	"strings"
	"testing"

	"github.com/pepabo/tazuna/pkg/prompt"
)

func TestYesORNo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		{"lowercase y", "y\n", true, false},
		{"uppercase Y", "Y\n", true, false},
		{"empty (default yes)", "\n", true, false},
		{"lowercase n", "n\n", false, false},
		{"uppercase N", "N\n", false, false},
		{"word no", "no\n", false, false},
		{"arbitrary", "anything\n", false, false},
		{"missing newline (EOF)", "y", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := strings.NewReader(tt.input)
			got, err := prompt.YesORNo(r, "Continue?")
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (got=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("YesORNo(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
