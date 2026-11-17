package tfstate

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// Step はアドレスを構成する 1 要素です。`.name` 形式の Name か、`[0]` / `["key"]`
// 形式の Index のいずれかを表します。
type Step struct {
	// Name は `.foo` 形式の要素名です。Index 要素のときは空文字列です。
	Name string
	// Index は `[...]` 形式の添字です。Name 要素のときは nil です。
	// 整数添字なら int64、文字列添字なら string が入ります。
	Index any
}

// IsIndex は Step が `[...]` 形式の添字かどうかを返します。
func (s Step) IsIndex() bool {
	return s.Index != nil
}

func (s Step) String() string {
	if !s.IsIndex() {
		return "." + s.Name
	}
	return "[" + formatIndexKey(s.Index) + "]"
}

// ResourceMode は managed resource か data source かを表します。
type ResourceMode string

const (
	// ResourceModeManaged は通常の resource ブロックです。
	ResourceModeManaged ResourceMode = "managed"
	// ResourceModeData は data ブロックです。
	ResourceModeData ResourceMode = "data"
)

// Address は tfstate 内の値を指すパース済みアドレスです。
//
// 対応している書式:
//
//	output.<name>[<path>]
//	[module.<name>[<index>].]...[data.]<type>.<name>[<index>].<attribute>[<path>]
//
// <path> は `.key` / `[0]` / `["key"]` の繰り返しで、output の値や resource
// の attributes の内側を辿ります。
type Address struct {
	// Output は output 参照のときの output 名です。resource 参照のときは空です。
	Output string
	// Module は resource が属する module のアドレスです (例: `module.vpc`,
	// `module.app["web"]`)。root module の resource のときは空です。tfstate の
	// resources[].module と同じ書式です。
	Module string
	// Mode は resource の mode (managed / data) です。
	Mode ResourceMode
	// Type は resource type (例: aws_vpc) です。
	Type string
	// Name は resource 名です。
	Name string
	// Index は resource instance の添字 (count なら int64、for_each なら string) です。
	// 添字なしのときは nil です。
	Index any
	// Path は output の値 / resource の attributes の内側を辿る要素列です。
	Path []Step

	raw string
}

// IsOutput は output 参照かどうかを返します。
func (a Address) IsOutput() bool {
	return a.Output != ""
}

// String はパース前のアドレス文字列を返します。
func (a Address) String() string {
	return a.raw
}

// resourceAddr は module / mode / type / name / index を terraform の
// アドレス書式で返します (エラーメッセージ用)。
func (a Address) resourceAddr() string {
	return resourceAddress(a.Module, a.Mode, a.Type, a.Name, a.Index)
}

func resourceAddress(module string, mode ResourceMode, typ, name string, index any) string {
	var b strings.Builder
	if module != "" {
		b.WriteString(module)
		b.WriteString(".")
	}
	if mode == ResourceModeData {
		b.WriteString("data.")
	}
	b.WriteString(typ)
	b.WriteString(".")
	b.WriteString(name)
	if index != nil {
		b.WriteString("[")
		b.WriteString(formatIndexKey(index))
		b.WriteString("]")
	}
	return b.String()
}

// ParseAddress はアドレス文字列をパースします。
func ParseAddress(s string) (Address, error) {
	steps, err := tokenize(s)
	if err != nil {
		return Address{}, errors.Wrapf(err, "invalid tfstate address %q", s)
	}
	addr, err := parseSteps(steps)
	if err != nil {
		return Address{}, errors.Wrapf(err, "invalid tfstate address %q", s)
	}
	addr.raw = s
	return addr, nil
}

func parseSteps(steps []Step) (Address, error) {
	if len(steps) == 0 {
		return Address{}, errors.New("address is empty")
	}

	next := func() (Step, bool) {
		if len(steps) == 0 {
			return Step{}, false
		}
		s := steps[0]
		steps = steps[1:]
		return s, true
	}
	expectName := func(what string) (string, error) {
		s, ok := next()
		if !ok {
			return "", errors.Errorf("%s is missing", what)
		}
		if s.IsIndex() {
			return "", errors.Errorf("%s is expected but got index %s", what, s)
		}
		return s.Name, nil
	}
	optionalIndex := func() any {
		if len(steps) > 0 && steps[0].IsIndex() {
			s, _ := next()
			return s.Index
		}
		return nil
	}

	if !steps[0].IsIndex() && steps[0].Name == "output" {
		next()
		name, err := expectName("output name")
		if err != nil {
			return Address{}, err
		}
		return Address{Output: name, Path: steps}, nil
	}

	var addr Address
	var modules []string
	for len(steps) > 0 && !steps[0].IsIndex() && steps[0].Name == "module" {
		next()
		name, err := expectName("module name")
		if err != nil {
			return Address{}, err
		}
		m := "module." + name
		if idx := optionalIndex(); idx != nil {
			m += "[" + formatIndexKey(idx) + "]"
		}
		modules = append(modules, m)
	}
	addr.Module = strings.Join(modules, ".")

	addr.Mode = ResourceModeManaged
	if len(steps) > 0 && !steps[0].IsIndex() && steps[0].Name == "data" {
		next()
		addr.Mode = ResourceModeData
	}

	typ, err := expectName("resource type")
	if err != nil {
		return Address{}, err
	}
	name, err := expectName("resource name")
	if err != nil {
		return Address{}, err
	}
	addr.Type = typ
	addr.Name = name
	addr.Index = optionalIndex()
	if len(steps) == 0 {
		return Address{}, errors.Errorf("attribute of %s is missing (e.g. %s.id)", addr.resourceAddr(), addr.resourceAddr())
	}
	if steps[0].IsIndex() {
		return Address{}, errors.Errorf("unexpected index %s after %s", steps[0], addr.resourceAddr())
	}
	addr.Path = steps
	return addr, nil
}

// tokenize はアドレス文字列を Step 列に分解します。
func tokenize(s string) ([]Step, error) {
	var steps []Step
	i := 0
	expectName := true // 先頭、または '.' の直後
	for i < len(s) {
		c := s[i]
		switch c {
		case '.':
			if expectName {
				return nil, errors.Errorf("unexpected '.' at position %d", i)
			}
			expectName = true
			i++
		case '[':
			if expectName {
				return nil, errors.Errorf("unexpected '[' at position %d", i)
			}
			idx, n, err := scanIndex(s[i:])
			if err != nil {
				return nil, errors.Wrapf(err, "at position %d", i)
			}
			steps = append(steps, Step{Index: idx})
			i += n
		default:
			if !expectName {
				return nil, errors.Errorf("unexpected character %q at position %d", c, i)
			}
			j := i
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			if j == i {
				return nil, errors.Errorf("unexpected character %q at position %d", c, i)
			}
			steps = append(steps, Step{Name: s[i:j]})
			expectName = false
			i = j
		}
	}
	if expectName {
		if len(s) == 0 {
			return nil, errors.New("address is empty")
		}
		return nil, errors.New("address must not end with '.'")
	}
	return steps, nil
}

// scanIndex は s の先頭にある `[...]` をパースし、添字と消費したバイト数を返します。
func scanIndex(s string) (any, int, error) {
	end := -1
	if len(s) > 1 && s[1] == '"' {
		// 文字列添字。エスケープされた '"' を飛ばして閉じ '"' を探す。
		for j := 2; j < len(s); j++ {
			if s[j] == '\\' {
				j++
				continue
			}
			if s[j] == '"' {
				if j+1 < len(s) && s[j+1] == ']' {
					end = j + 1
				}
				break
			}
		}
		if end < 0 {
			return nil, 0, errors.New("unterminated string index")
		}
		key, err := strconv.Unquote(s[1:end])
		if err != nil {
			return nil, 0, errors.Wrap(err, "invalid string index")
		}
		return key, end + 1, nil
	}

	end = strings.IndexByte(s, ']')
	if end < 0 {
		return nil, 0, errors.New("unterminated index")
	}
	body := s[1:end]
	n, err := strconv.ParseInt(body, 10, 64)
	if err != nil || n < 0 {
		return nil, 0, errors.Errorf("index %q must be a non-negative integer or a double-quoted string", body)
	}
	return n, end + 1, nil
}

func isIdentChar(c byte) bool {
	return c == '_' || c == '-' ||
		('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}

// formatIndexKey は index_key を terraform のアドレス書式で返します。
func formatIndexKey(k any) string {
	switch v := k.(type) {
	case string:
		return strconv.Quote(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprint(v)
	}
}
