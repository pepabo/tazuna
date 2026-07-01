package tmpl_test

import (
	"strings"
	"testing"

	"github.com/pepabo/tazuna/pkg/tmpl"
)

func TestRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		raw         string
		data        tmpl.Data
		want        string
		expectErr   bool
		errContains string
	}{
		{
			name: "environment is substituted",
			raw:  "context: {{ .Environment }}-cluster",
			data: tmpl.Data{Environment: "prod"},
			want: "context: prod-cluster",
		},
		{
			name: "empty environment renders empty string",
			raw:  "context: {{ .Environment }}",
			data: tmpl.Data{Environment: ""},
			want: "context: ",
		},
		{
			name: "no template directive is returned as-is",
			raw:  "context: static",
			data: tmpl.Data{Environment: "prod"},
			want: "context: static",
		},
		{
			name:        "unknown field is a render error",
			raw:         "context: {{ .Unknown }}",
			data:        tmpl.Data{Environment: "prod"},
			expectErr:   true,
			errContains: "render",
		},
		{
			name:        "malformed template is a parse error",
			raw:         "context: {{ .Environment ",
			data:        tmpl.Data{Environment: "prod"},
			expectErr:   true,
			errContains: "parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tmpl.Render("test.yaml", []byte(tt.raw), tt.data)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error but got nil (got=%q)", string(got))
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got: %s", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Render() = %q, want %q", string(got), tt.want)
			}
		})
	}
}
