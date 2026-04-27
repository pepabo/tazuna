package hint

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"gopkg.in/yaml.v3"
)

const HintFileName = "tazuna.hint.yaml"

// knownFormats は有効なHintFormat値の集合です。
var knownFormats = map[v1.HintFormat]bool{
	v1.HintFormatHostname: true,
	v1.HintFormatURL:      true,
	v1.HintFormatEmail:    true,
	v1.HintFormatIP:       true,
	v1.HintFormatCIDR:     true,
	v1.HintFormatUUID:     true,
	v1.HintFormatSemver:   true,
	v1.HintFormatDatetime: true,
}

// フォーマット検証用の正規表現
var (
	// hostnameRegexp はRFC 952/1123に準拠したホスト名パターンです。
	// 各ラベルは英数字で始まり英数字で終わる必要があり、ハイフンを含むことができます。
	hostnameRegexp = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

	// emailRegexp は簡易的なemail検証パターンです（user@domain形式）。
	emailRegexp = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

	// uuidRegexp はRFC 4122形式のUUIDパターンです。
	uuidRegexp = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// semverRegexp はセマンティックバージョニングパターンです（v接頭辞はオプション）。
	semverRegexp = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(-[0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*)?(\+[0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*)?$`)
)

// LoadHintFile はディレクトリからtazuna.hint.yamlを読み込みます。
// ファイルが存在しない場合は (nil, nil) を返します（後方互換）。
func LoadHintFile(dir string) (*v1.TazunaHint, error) {
	path := filepath.Join(dir, HintFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to read hint file %s", path)
	}

	var hint v1.TazunaHint
	if err := yaml.Unmarshal(data, &hint); err != nil {
		return nil, errors.Wrapf(err, "failed to parse hint file %s", path)
	}

	return &hint, nil
}

// ValidateHint はhintファイルの妥当性を検証します。
// スキーマレベルの検証を行い、以下を確認します:
//   - 各varの型が有効であること
//   - required:trueとdefaultの併用がないこと
//   - formatがstring型のvarにのみ指定されていること
//   - formatの値が既知であること
//   - required_with/required_withoutの参照先がvarsに存在すること
//   - required:trueとrequired_with/required_withoutの併用がないこと
//     （required:trueは常に必須、required_with/required_withoutは条件付き必須であり矛盾するため）
//   - rulesのtypeが既知であること
//   - rulesのvarsが2件以上であること
//   - rulesの参照先がvarsに存在すること
func ValidateHint(hint *v1.TazunaHint) error {
	if hint == nil {
		return nil
	}

	for name, v := range hint.Vars {
		switch v.Type {
		case v1.HintVarTypeString, v1.HintVarTypeSlice, v1.HintVarTypeMap:
			// ok
		default:
			return errors.Errorf("hint var %q has invalid type %q (must be string, slice, or map)", name, v.Type)
		}

		if v.Required && v.Default != nil {
			return errors.Errorf("hint var %q has required:true with a default value, which is contradictory", name)
		}

		// formatはstring型のみ有効
		if v.Format != "" {
			if v.Type != v1.HintVarTypeString {
				return errors.Errorf("hint var %q has format %q but type is %q (format is only valid for string type)", name, v.Format, v.Type)
			}
			if !knownFormats[v.Format] {
				return errors.Errorf("hint var %q has unknown format %q", name, v.Format)
			}
		}

		// required:trueとrequired_with/required_withoutは矛盾する
		if v.Required && len(v.RequiredWith) > 0 {
			return errors.Errorf("hint var %q has required:true with required_with, which is contradictory (required:true means always required, required_with means conditionally required)", name)
		}
		if v.Required && len(v.RequiredWithout) > 0 {
			return errors.Errorf("hint var %q has required:true with required_without, which is contradictory (required:true means always required, required_without means conditionally required)", name)
		}

		// required_withの参照先がvarsに存在するか検証
		for _, ref := range v.RequiredWith {
			if _, ok := hint.Vars[ref]; !ok {
				return errors.Errorf("hint var %q has required_with referencing non-existent var %q", name, ref)
			}
		}

		// required_withoutの参照先がvarsに存在するか検証
		for _, ref := range v.RequiredWithout {
			if _, ok := hint.Vars[ref]; !ok {
				return errors.Errorf("hint var %q has required_without referencing non-existent var %q", name, ref)
			}
		}
	}

	// rulesの検証
	for i, rule := range hint.Rules {
		switch rule.Type {
		case v1.HintRuleTypeOneofRequired:
			// ok
		default:
			return errors.Errorf("rule[%d] has unknown type %q", i, rule.Type)
		}

		if len(rule.Vars) < 2 {
			return errors.Errorf("rule[%d] (type %q) must have at least 2 vars, got %d", i, rule.Type, len(rule.Vars))
		}

		for _, ref := range rule.Vars {
			if _, ok := hint.Vars[ref]; !ok {
				return errors.Errorf("rule[%d] (type %q) references non-existent var %q", i, rule.Type, ref)
			}
		}
	}

	return nil
}

// ValidateVarsAgainstHint はtazuna.yamlのvarsがhintの型定義と互換性があるか検証します。
// hintに定義されていないvarはそのまま通します。
func ValidateVarsAgainstHint(hint *v1.TazunaHint, vars map[string]v1.HelmFileVar) error {
	if hint == nil {
		return nil
	}

	for name, hv := range hint.Vars {
		v, ok := vars[name]
		if !ok {
			continue
		}

		switch hv.Type {
		case v1.HintVarTypeString:
			if v.StaticSlice != nil || v.StaticMap != nil {
				return errors.Errorf("hint var %q expects type string, but got slice or map value", name)
			}
		case v1.HintVarTypeSlice:
			if v.From == v1.HelmFileVarFromStatic && v.StaticSlice == nil {
				return errors.Errorf("hint var %q expects type slice, but got non-slice static value", name)
			}
		case v1.HintVarTypeMap:
			if v.From == v1.HelmFileVarFromStatic && v.StaticMap == nil {
				return errors.Errorf("hint var %q expects type map, but got non-map static value", name)
			}
		}
	}

	return nil
}

// MergeVarsWithHint はhintのデフォルト値注入と必須チェックを行います。
// resolvedVarsは既にConstructHelmfileVarsで解決済みのvar値です。
//
// 「提供済み」の判定は元のresolvedVarsを参照します。
// ゼロ値注入後のresultを参照すると、ゼロ値が注入されたvarも「提供済み」と
// 判定されてしまうため、正確な条件付き必須チェックができません。
func MergeVarsWithHint(hint *v1.TazunaHint, resolvedVars map[string]any) (map[string]any, error) {
	if hint == nil {
		return resolvedVars, nil
	}

	result := make(map[string]any, len(resolvedVars))
	for k, v := range resolvedVars {
		result[k] = v
	}

	for name, hv := range hint.Vars {
		if _, ok := result[name]; ok {
			continue
		}

		if hv.Required {
			return nil, errors.Errorf("hint var %q is required but not provided", name)
		}

		if hv.Default != nil {
			result[name] = hv.Default
			continue
		}

		// ゼロ値注入
		switch hv.Type {
		case v1.HintVarTypeString:
			result[name] = ""
		case v1.HintVarTypeSlice:
			result[name] = []any{}
		case v1.HintVarTypeMap:
			result[name] = map[string]any{}
		}
	}

	// required_with: 参照先varがユーザーから提供済みなら、このvarも提供必須
	for name, hv := range hint.Vars {
		if len(hv.RequiredWith) == 0 {
			continue
		}
		// 自身が提供済みなら問題なし
		if _, ok := resolvedVars[name]; ok {
			continue
		}
		// 参照先のいずれかが提供済みなら、このvarは必須
		for _, ref := range hv.RequiredWith {
			if _, ok := resolvedVars[ref]; ok {
				return nil, errors.Errorf("hint var %q is required because %q is provided (required_with)", name, ref)
			}
		}
	}

	// required_without: 参照先varが全て未提供なら、このvarは提供必須
	for name, hv := range hint.Vars {
		if len(hv.RequiredWithout) == 0 {
			continue
		}
		// 自身が提供済みなら問題なし
		if _, ok := resolvedVars[name]; ok {
			continue
		}
		// 参照先が全て未提供なら、このvarは必須
		allMissing := true
		for _, ref := range hv.RequiredWithout {
			if _, ok := resolvedVars[ref]; ok {
				allMissing = false
				break
			}
		}
		if allMissing {
			return nil, errors.Errorf("hint var %q is required because none of %v are provided (required_without)", name, hv.RequiredWithout)
		}
	}

	// format検証: 値が非空文字列ならフォーマット検証を実行
	for name, hv := range hint.Vars {
		if hv.Format == "" {
			continue
		}
		val, ok := result[name]
		if !ok {
			continue
		}
		strVal, ok := val.(string)
		if !ok || strVal == "" {
			continue
		}
		if err := validateFormat(hv.Format, strVal); err != nil {
			return nil, errors.Errorf("hint var %q value %q does not match format %q: %s", name, strVal, hv.Format, err)
		}
	}

	// oneof_required: ルール内のvarのうち少なくとも1つがユーザーから提供されているか検証
	for i, rule := range hint.Rules {
		if rule.Type != v1.HintRuleTypeOneofRequired {
			continue
		}
		anyProvided := false
		for _, ref := range rule.Vars {
			if _, ok := resolvedVars[ref]; ok {
				anyProvided = true
				break
			}
		}
		if !anyProvided {
			msg := rule.Message
			if msg == "" {
				msg = fmt.Sprintf("at least one of %v must be provided", rule.Vars)
			}
			return nil, errors.Errorf("rule[%d] (oneof_required): %s", i, msg)
		}
	}

	return result, nil
}

// validateFormat は指定されたフォーマットに対して値を検証します。
//
// 各フォーマットの検証方法:
//   - hostname: 正規表現によるRFC 952/1123準拠チェック
//   - url: net/url.ParseRequestURIでの解析 + schemeの存在確認
//   - email: 正規表現によるuser@domain形式チェック
//   - ip: net.ParseIPによるIPv4/IPv6解析
//   - cidr: net.ParseCIDRによるCIDR表記解析
//   - uuid: 正規表現によるRFC 4122形式チェック
//   - semver: 正規表現によるセマンティックバージョニングチェック（v接頭辞オプション）
//   - datetime: time.Parse(time.RFC3339, value)による解析
func validateFormat(format v1.HintFormat, value string) error {
	switch format {
	case v1.HintFormatHostname:
		if !hostnameRegexp.MatchString(value) {
			return errors.New("invalid hostname")
		}
	case v1.HintFormatURL:
		u, err := url.ParseRequestURI(value)
		if err != nil {
			return errors.New("invalid URL")
		}
		if u.Scheme == "" {
			return errors.New("invalid URL: missing scheme")
		}
	case v1.HintFormatEmail:
		if !emailRegexp.MatchString(value) {
			return errors.New("invalid email")
		}
	case v1.HintFormatIP:
		if net.ParseIP(value) == nil {
			return errors.New("invalid IP address")
		}
	case v1.HintFormatCIDR:
		if _, _, err := net.ParseCIDR(value); err != nil {
			return errors.New("invalid CIDR")
		}
	case v1.HintFormatUUID:
		if !uuidRegexp.MatchString(value) {
			return errors.New("invalid UUID")
		}
	case v1.HintFormatSemver:
		if !semverRegexp.MatchString(value) {
			return errors.New("invalid semver")
		}
	case v1.HintFormatDatetime:
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return errors.New("invalid datetime (expected RFC3339)")
		}
	default:
		return errors.Errorf("unknown format %q", format)
	}
	return nil
}
