package runner

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ColorMode は plan 出力への ANSI カラー付与モードを表す。
// - ColorModeAuto: stdout が TTY のとき有効、NO_COLOR が設定されているときは無効
// - ColorModeOn:   常に有効
// - ColorModeOff:  常に無効
type ColorMode string

const (
	ColorModeAuto ColorMode = "auto"
	ColorModeOn   ColorMode = "on"
	ColorModeOff  ColorMode = "off"
)

// ParseColorMode は "auto" / "on" / "off" のいずれかを ColorMode に変換する。
// 空文字列は auto として扱う。
func ParseColorMode(s string) (ColorMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(ColorModeAuto):
		return ColorModeAuto, nil
	case string(ColorModeOn):
		return ColorModeOn, nil
	case string(ColorModeOff):
		return ColorModeOff, nil
	default:
		return "", fmt.Errorf("invalid color mode %q (expected auto|on|off)", s)
	}
}

// shouldColorize は ColorMode と書き出し先から実際に色付けするかを判定する。
func shouldColorize(mode ColorMode, w io.Writer) bool {
	switch mode {
	case ColorModeOn:
		return true
	case ColorModeOff:
		return false
	default:
		return planColorEnabled(w)
	}
}

// plan 出力の diff 色付けに使う ANSI エスケープシーケンス。
//   - 行全体: - 行は赤文字、+ 行は緑文字 (背景色なし)
//   - 差分文字: - 行は赤背景+黒文字、+ 行は緑背景+黒文字
const (
	ansiReset      = "\x1b[0m"
	ansiRedFG      = "\x1b[31m"
	ansiGrnFG      = "\x1b[32m"
	ansiRedBGBlack = "\x1b[41;30m"
	ansiGrnBGBlack = "\x1b[42;30m"
)

// planColorEnabled は plan の diff 出力に ANSI カラーを付けるか判定する。
// NO_COLOR が設定されているとき、または書き出し先が TTY でないときは false。
func planColorEnabled(w io.Writer) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// colorizeDiff は util/diff.Diff の unified diff 出力に色付けする。
//   - "-" で始まる行は薄い赤背景、"+" で始まる行は薄い緑背景で表示する
//   - 隣接する - / + ブロックを 1:1 でペアにして共通 prefix/suffix を計算し、
//     実際に変化した中央部分を濃い背景で強調する
//
// "---" / "+++" のヘッダ行と "@@" などは色付けの対象外。
func colorizeDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		line := lines[i]
		if !isMinusLine(line) && !isPlusLine(line) {
			out = append(out, line)
			i++
			continue
		}

		minusStart := i
		for i < len(lines) && isMinusLine(lines[i]) {
			i++
		}
		minus := lines[minusStart:i]

		plusStart := i
		for i < len(lines) && isPlusLine(lines[i]) {
			i++
		}
		plus := lines[plusStart:i]

		n := len(minus)
		if len(plus) > n {
			n = len(plus)
		}
		for j := 0; j < n; j++ {
			hasM := j < len(minus)
			hasP := j < len(plus)
			switch {
			case hasM && hasP:
				m, p := highlightLinePair(minus[j], plus[j])
				out = append(out, m, p)
			case hasM:
				out = append(out, wrapDiffLine(minus[j], -1, -1, ansiRedFG, ansiRedBGBlack))
			case hasP:
				out = append(out, wrapDiffLine(plus[j], -1, -1, ansiGrnFG, ansiGrnBGBlack))
			}
		}
	}
	return strings.Join(out, "\n")
}

func isMinusLine(s string) bool {
	return strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "---")
}

func isPlusLine(s string) bool {
	return strings.HasPrefix(s, "+") && !strings.HasPrefix(s, "+++")
}

// highlightLinePair は - と + の 1 行ペアに対し、共通する先頭/末尾を除いた
// 中央部分だけを濃い背景で強調した色付き文字列を返す。
func highlightLinePair(minus, plus string) (string, string) {
	mBody := minus[1:]
	pBody := plus[1:]

	prefixLen := 0
	maxPrefix := len(mBody)
	if len(pBody) < maxPrefix {
		maxPrefix = len(pBody)
	}
	for prefixLen < maxPrefix && mBody[prefixLen] == pBody[prefixLen] {
		prefixLen++
	}

	suffixLen := 0
	maxSuffix := len(mBody) - prefixLen
	if len(pBody)-prefixLen < maxSuffix {
		maxSuffix = len(pBody) - prefixLen
	}
	for suffixLen < maxSuffix && mBody[len(mBody)-1-suffixLen] == pBody[len(pBody)-1-suffixLen] {
		suffixLen++
	}

	// minus / plus 行内での「変化した範囲」の [start, end) を計算する。
	// 先頭記号 (- / +) の分だけ +1 する。
	mStart := 1 + prefixLen
	mEnd := 1 + len(mBody) - suffixLen
	pStart := 1 + prefixLen
	pEnd := 1 + len(pBody) - suffixLen

	return wrapDiffLine(minus, mStart, mEnd, ansiRedFG, ansiRedBGBlack),
		wrapDiffLine(plus, pStart, pEnd, ansiGrnFG, ansiGrnBGBlack)
}

// wrapDiffLine は line 全体を base 色で囲み、[start, end) の範囲を emphasis 色で強調する。
// start < 0 / start >= end / end > len(line) の場合は強調なしで base 色のみを適用する。
func wrapDiffLine(line string, start, end int, base, emphasis string) string {
	if start < 0 || start >= end || end > len(line) {
		return base + line + ansiReset
	}
	var b strings.Builder
	b.Grow(len(line) + len(base)*2 + len(emphasis) + len(ansiReset)*2)
	b.WriteString(base)
	b.WriteString(line[:start])
	b.WriteString(emphasis)
	b.WriteString(line[start:end])
	b.WriteString(ansiReset)
	b.WriteString(base)
	b.WriteString(line[end:])
	b.WriteString(ansiReset)
	return b.String()
}
