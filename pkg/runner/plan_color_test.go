package runner

import (
	"strings"
	"testing"
)

// TestColorizeDiff_PairedLinesHighlightMiddle は - / + のペア行で
// 共通 prefix / suffix を除いた中央部分だけが背景色付きで強調され、
// 行全体は赤/緑の前景色になることを検証する。
func TestColorizeDiff_PairedLinesHighlightMiddle(t *testing.T) {
	in := "-  \"replicas\": \"1\"\n+  \"replicas\": \"3\"\n"
	got := colorizeDiff(in)

	if !strings.Contains(got, ansiRedFG+"-  \"replicas\": \"") {
		t.Errorf("- 行の共通 prefix が赤文字になっていない: %q", got)
	}
	if !strings.Contains(got, ansiGrnFG+"+  \"replicas\": \"") {
		t.Errorf("+ 行の共通 prefix が緑文字になっていない: %q", got)
	}
	if !strings.Contains(got, ansiRedBGBlack+"1"+ansiReset) {
		t.Errorf("- 行の変化文字 '1' が赤背景+黒文字で強調されていない: %q", got)
	}
	if !strings.Contains(got, ansiGrnBGBlack+"3"+ansiReset) {
		t.Errorf("+ 行の変化文字 '3' が緑背景+黒文字で強調されていない: %q", got)
	}
}

// TestColorizeDiff_HeaderLinesUntouched は "---" / "+++" / "@@" などの
// メタ行に色が付かないことを検証する。
func TestColorizeDiff_HeaderLinesUntouched(t *testing.T) {
	in := "--- a\n+++ b\n@@ -1,1 +1,1 @@\n"
	got := colorizeDiff(in)
	if strings.Contains(got, "\x1b[") {
		t.Errorf("ヘッダ行に ANSI エスケープが混入している: %q", got)
	}
}

// TestColorizeDiff_UnpairedMinusLine は + 側に対応がない - 行が
// 単に赤文字で表示され、強調背景を持たないことを検証する。
func TestColorizeDiff_UnpairedMinusLine(t *testing.T) {
	in := "-removed only\n context\n"
	got := colorizeDiff(in)
	if !strings.Contains(got, ansiRedFG+"-removed only"+ansiReset) {
		t.Errorf("単独の - 行が赤文字で表示されていない: %q", got)
	}
	if strings.Contains(got, ansiRedBGBlack) {
		t.Errorf("単独の - 行に強調背景が付いてはいけない: %q", got)
	}
}
