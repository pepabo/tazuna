package runner

import (
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
)

func TestMatchesTags(t *testing.T) {
	t.Parallel()
	m := v1.Manifest{Tags: []string{"web", "batch"}}

	tests := []struct {
		name   string
		filter []string
		want   bool
	}{
		{"empty filter matches all", nil, true},
		{"matching tag", []string{"web"}, true},
		{"one of filter matches (OR)", []string{"nomatch", "batch"}, true},
		{"no match", []string{"db"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := matchesTags(m, tt.filter); got != tt.want {
				t.Errorf("matchesTags(%v) = %v, want %v", tt.filter, got, tt.want)
			}
		})
	}

	t.Run("manifest without tags does not match a filter", func(t *testing.T) {
		t.Parallel()
		if matchesTags(v1.Manifest{}, []string{"web"}) {
			t.Error("expected no match for untagged manifest")
		}
	})
}
