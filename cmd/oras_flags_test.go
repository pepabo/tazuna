package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newTestCmdWithORASFlags() *cobra.Command {
	c := &cobra.Command{Use: "test"}
	addORASPullFlags(c)
	return c
}

func TestBuildORASPullOptions_Default(t *testing.T) {
	t.Parallel()
	c := newTestCmdWithORASFlags()
	if err := c.ParseFlags(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	opts, err := buildORASPullOptions(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.NoCache || opts.Offline {
		t.Errorf("expected zero options, got %+v", opts)
	}
}

func TestBuildORASPullOptions_NoCacheOnly(t *testing.T) {
	t.Parallel()
	c := newTestCmdWithORASFlags()
	if err := c.ParseFlags([]string{"--no-cache"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	opts, err := buildORASPullOptions(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.NoCache || opts.Offline {
		t.Errorf("expected NoCache only, got %+v", opts)
	}
}

func TestBuildORASPullOptions_OfflineOnly(t *testing.T) {
	t.Parallel()
	c := newTestCmdWithORASFlags()
	if err := c.ParseFlags([]string{"--offline"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	opts, err := buildORASPullOptions(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.NoCache || !opts.Offline {
		t.Errorf("expected Offline only, got %+v", opts)
	}
}

func TestBuildORASPullOptions_MutualExclusion(t *testing.T) {
	t.Parallel()
	c := newTestCmdWithORASFlags()
	if err := c.ParseFlags([]string{"--no-cache", "--offline"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err := buildORASPullOptions(c)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--no-cache") || !strings.Contains(err.Error(), "--offline") {
		t.Errorf("error message should mention both flags: %v", err)
	}
}
