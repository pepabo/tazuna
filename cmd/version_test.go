package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestVersionCmd_PrintsTemplate(t *testing.T) {
	prevVersion, prevCommit, prevDate := versionString, commitString, dateString
	t.Cleanup(func() {
		SetVersionInfo(prevVersion, prevCommit, prevDate)
	})

	SetVersionInfo("v1.2.3", "abcdef0", "2026-05-18T00:00:00Z")

	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionCmd.SetErr(&buf)
	versionCmd.SetContext(context.Background())
	if err := versionCmd.RunE(versionCmd, []string{}); err != nil {
		t.Fatalf("version RunE: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"v1.2.3", "abcdef0", "2026-05-18T00:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q: %s", want, got)
		}
	}
}

func TestSetVersionInfo_WiresRootVersion(t *testing.T) {
	prevVersion, prevCommit, prevDate := versionString, commitString, dateString
	t.Cleanup(func() {
		SetVersionInfo(prevVersion, prevCommit, prevDate)
	})

	SetVersionInfo("v9.9.9", "deadbee", "2026-01-01")
	if rootCmd.Version != "v9.9.9" {
		t.Errorf("rootCmd.Version = %q, want v9.9.9", rootCmd.Version)
	}
}

func TestSetVersionInfo_EmptyArgsAreIgnored(t *testing.T) {
	prevVersion, prevCommit, prevDate := versionString, commitString, dateString
	t.Cleanup(func() {
		SetVersionInfo(prevVersion, prevCommit, prevDate)
	})

	SetVersionInfo("v1.0.0", "cafebab", "2026-03-01")
	SetVersionInfo("", "", "")
	if versionString != "v1.0.0" || commitString != "cafebab" || dateString != "2026-03-01" {
		t.Errorf("empty args overwrote previous values: %s/%s/%s",
			versionString, commitString, dateString)
	}
}
