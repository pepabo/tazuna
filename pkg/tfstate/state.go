// Package tfstate は Terraform の state (tfstate) を各種 backend から読み出し、
// アドレス指定で値を取り出す機能を提供します。
//
// 構成:
//   - Backend: tfstate の生バイト列を取得する抽象 (s3 / local)。新しい backend は
//     BackendFactory を 1 つ実装して Factories に登録するだけで追加できます。
//   - State: tfstate (format version 4) のパース結果と Lookup。
//   - Resolver: tazuna.yaml の spec.tfstates[] を名前で引き、state の読み込みを
//     ソースごとに 1 回へキャッシュします。
package tfstate

import (
	"bytes"
	"encoding/json"
	"math"
	"sort"
	"strconv"

	"github.com/cockroachdb/errors"
)

// SupportedStateVersion は対応している tfstate の format version です。
// Terraform 0.12 以降 (OpenTofu を含む) はすべて version 4 を出力します。
const SupportedStateVersion = 4

// State はパース済みの tfstate です。
type State struct {
	Version          int
	TerraformVersion string
	Serial           int64
	Lineage          string
	Outputs          map[string]Output
	Resources        []Resource
}

// Output は tfstate の outputs の 1 エントリです。
type Output struct {
	Value     any
	Sensitive bool
}

// Resource は tfstate の resources の 1 エントリです。
type Resource struct {
	Module    string
	Mode      ResourceMode
	Type      string
	Name      string
	Instances []Instance
}

// Instance は resource の 1 インスタンスです。
type Instance struct {
	// IndexKey は count なら int64、for_each なら string、どちらでもなければ nil です。
	IndexKey   any
	Attributes map[string]any
}

type rawState struct {
	Version          int                  `json:"version"`
	TerraformVersion string               `json:"terraform_version"`
	Serial           int64                `json:"serial"`
	Lineage          string               `json:"lineage"`
	Outputs          map[string]rawOutput `json:"outputs"`
	Resources        []rawResource        `json:"resources"`
}

type rawOutput struct {
	Value     any  `json:"value"`
	Sensitive bool `json:"sensitive"`
}

type rawResource struct {
	Module    string        `json:"module"`
	Mode      string        `json:"mode"`
	Type      string        `json:"type"`
	Name      string        `json:"name"`
	Instances []rawInstance `json:"instances"`
}

type rawInstance struct {
	IndexKey   any            `json:"index_key"`
	Attributes map[string]any `json:"attributes"`
}

// Parse は tfstate の JSON をパースします。format version 4 以外はエラーです。
func Parse(data []byte) (*State, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	// 数値を float64 に落とすと大きな整数 (AWS アカウント ID 等) の精度が失われる
	// ため、json.Number で受けてから normalize する。
	dec.UseNumber()

	var raw rawState
	if err := dec.Decode(&raw); err != nil {
		return nil, errors.Wrap(err, "failed to decode tfstate as JSON")
	}
	if raw.Version != SupportedStateVersion {
		return nil, errors.Errorf("unsupported tfstate format version %d (only version %d, written by Terraform >= 0.12, is supported)",
			raw.Version, SupportedStateVersion)
	}

	st := &State{
		Version:          raw.Version,
		TerraformVersion: raw.TerraformVersion,
		Serial:           raw.Serial,
		Lineage:          raw.Lineage,
		Outputs:          make(map[string]Output, len(raw.Outputs)),
		Resources:        make([]Resource, 0, len(raw.Resources)),
	}
	for name, o := range raw.Outputs {
		st.Outputs[name] = Output{Value: normalize(o.Value), Sensitive: o.Sensitive}
	}
	for _, r := range raw.Resources {
		res := Resource{
			Module:    r.Module,
			Mode:      ResourceMode(r.Mode),
			Type:      r.Type,
			Name:      r.Name,
			Instances: make([]Instance, 0, len(r.Instances)),
		}
		for _, in := range r.Instances {
			attrs, _ := normalize(in.Attributes).(map[string]any)
			res.Instances = append(res.Instances, Instance{
				IndexKey:   normalize(in.IndexKey),
				Attributes: attrs,
			})
		}
		st.Resources = append(st.Resources, res)
	}
	return st, nil
}

// normalize は json.Number を int64 (整数で表現できる場合) か float64 に変換し、
// map / slice を再帰的に辿ります。
func normalize(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := strconv.ParseInt(t.String(), 10, 64); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normalize(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalize(e)
		}
		return out
	default:
		return v
	}
}

// LookupString は Address をパースして Lookup します。
func (s *State) LookupString(address string) (any, error) {
	addr, err := ParseAddress(address)
	if err != nil {
		return nil, err
	}
	return s.Lookup(addr)
}

// Lookup は addr が指す値を返します。見つからない場合は、候補 (output 名や
// resource アドレス) を含むエラーを返します。エラーメッセージには値そのものは
// 含めません。
func (s *State) Lookup(addr Address) (any, error) {
	if addr.IsOutput() {
		o, ok := s.Outputs[addr.Output]
		if !ok {
			return nil, errors.Errorf("output %q not found in tfstate (available outputs: %s)",
				addr.Output, candidates(s.outputNames()))
		}
		v, err := walk(o.Value, addr.Path)
		if err != nil {
			return nil, errors.Wrapf(err, "output %q", addr.Output)
		}
		return v, nil
	}

	inst, err := s.findInstance(addr)
	if err != nil {
		return nil, err
	}
	v, err := walk(inst.Attributes, addr.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "resource %s", addr.resourceAddr())
	}
	return v, nil
}

func (s *State) findInstance(addr Address) (*Instance, error) {
	for ri := range s.Resources {
		r := &s.Resources[ri]
		if r.Module != addr.Module || r.Mode != addr.Mode || r.Type != addr.Type || r.Name != addr.Name {
			continue
		}
		for ii := range r.Instances {
			in := &r.Instances[ii]
			if indexKeyEqual(in.IndexKey, addr.Index) {
				return in, nil
			}
		}
		keys := make([]string, 0, len(r.Instances))
		for _, in := range r.Instances {
			if in.IndexKey == nil {
				keys = append(keys, "(no index)")
			} else {
				keys = append(keys, "["+formatIndexKey(in.IndexKey)+"]")
			}
		}
		if addr.Index == nil {
			return nil, errors.Errorf("resource %s uses count or for_each; specify an index (available: %s)",
				addr.resourceAddr(), candidates(keys))
		}
		return nil, errors.Errorf("resource instance %s not found in tfstate (available indexes: %s)",
			addr.resourceAddr(), candidates(keys))
	}
	return nil, errors.Errorf("resource %s not found in tfstate (available resources: %s)",
		resourceAddress(addr.Module, addr.Mode, addr.Type, addr.Name, nil), candidates(s.resourceAddrs()))
}

func indexKeyEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a == b
}

// walk は v から path を辿ります。
func walk(v any, path []Step) (any, error) {
	cur := v
	for i, st := range path {
		where := pathString(path[:i+1])
		switch c := cur.(type) {
		case map[string]any:
			key := st.Name
			if st.IsIndex() {
				k, ok := st.Index.(string)
				if !ok {
					return nil, errors.Errorf("%s: cannot use integer index on an object", where)
				}
				key = k
			}
			next, ok := c[key]
			if !ok {
				return nil, errors.Errorf("%s: key %q not found (available keys: %s)", where, key, candidates(mapKeys(c)))
			}
			cur = next
		case []any:
			idx, ok := listIndex(st)
			if !ok {
				return nil, errors.Errorf("%s: a list requires an integer index", where)
			}
			if idx < 0 || idx >= int64(len(c)) {
				return nil, errors.Errorf("%s: index %d out of range (length %d)", where, idx, len(c))
			}
			cur = c[idx]
		case nil:
			return nil, errors.Errorf("%s: cannot traverse into null", where)
		default:
			return nil, errors.Errorf("%s: cannot traverse into a %s value", where, kindOf(cur))
		}
	}
	return cur, nil
}

// listIndex は list に対する添字を返します。`[0]` のほか `.0` も許容します。
func listIndex(st Step) (int64, bool) {
	if st.IsIndex() {
		i, ok := st.Index.(int64)
		return i, ok
	}
	i, err := strconv.ParseInt(st.Name, 10, 64)
	if err != nil {
		return 0, false
	}
	return i, true
}

func pathString(path []Step) string {
	var b bytes.Buffer
	for _, s := range path {
		b.WriteString(s.String())
	}
	if b.Len() > 0 && b.Bytes()[0] == '.' {
		return b.String()[1:]
	}
	return b.String()
}

func kindOf(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "bool"
	case int64, float64:
		return "number"
	default:
		return "scalar"
	}
}

func (s *State) outputNames() []string {
	return mapKeysOf(s.Outputs)
}

func (s *State) resourceAddrs() []string {
	out := make([]string, 0, len(s.Resources))
	for _, r := range s.Resources {
		out = append(out, resourceAddress(r.Module, r.Mode, r.Type, r.Name, nil))
	}
	sort.Strings(out)
	return out
}

func mapKeys(m map[string]any) []string {
	return mapKeysOf(m)
}

func mapKeysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// maxCandidates はエラーメッセージに載せる候補の最大数です。
const maxCandidates = 20

func candidates(names []string) string {
	if len(names) == 0 {
		return "<none>"
	}
	if len(names) > maxCandidates {
		return joinComma(names[:maxCandidates]) + ", ... (" + strconv.Itoa(len(names)-maxCandidates) + " more)"
	}
	return joinComma(names)
}

func joinComma(names []string) string {
	var b bytes.Buffer
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(n)
	}
	return b.String()
}

// FormatValue は tfstate の値を文字列に変換します。GenesisSecret の値や
// template 関数 `tfstate` の出力に使います。
//   - string はそのまま
//   - number / bool は文字列化 (整数は指数表記にならない)
//   - list / object は JSON
//   - null はエラー
func FormatValue(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", errors.New("value is null")
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case float64:
		if math.IsInf(t, 0) || math.IsNaN(t) {
			return "", errors.Errorf("unsupported number %v", t)
		}
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return "", errors.Wrap(err, "failed to encode value as JSON")
		}
		return string(b), nil
	}
}
