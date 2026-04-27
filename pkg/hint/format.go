package hint

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"gopkg.in/yaml.v3"
)

// FormatHuman はhintをhuman-readableな形式で出力します。
func FormatHuman(hint *v1.TazunaHint, target string) string {
	if hint == nil || len(hint.Vars) == 0 {
		return fmt.Sprintf("No vars defined for %s\n", target)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Vars for %s:\n\n", target)

	names := make([]string, 0, len(hint.Vars))
	for name := range hint.Vars {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		v := hint.Vars[name]
		required := "optional"
		if v.Required {
			required = "required"
		}

		typeInfo := string(v.Type)
		if v.Format != "" {
			typeInfo = fmt.Sprintf("%s, format:%s", typeInfo, v.Format)
		}
		fmt.Fprintf(&b, "  %s (%s, %s)\n", name, typeInfo, required)

		if v.Description != "" {
			fmt.Fprintf(&b, "    %s\n", v.Description)
		}
		if v.Default != nil {
			fmt.Fprintf(&b, "    default: %v\n", v.Default)
		}
		if len(v.RequiredWith) > 0 {
			fmt.Fprintf(&b, "    required_with: %v\n", v.RequiredWith)
		}
		if len(v.RequiredWithout) > 0 {
			fmt.Fprintf(&b, "    required_without: %v\n", v.RequiredWithout)
		}
	}

	if len(hint.Rules) > 0 {
		b.WriteString("\nRules:\n")
		for i, rule := range hint.Rules {
			msg := rule.Message
			if msg == "" {
				msg = "(no message)"
			}
			fmt.Fprintf(&b, "  [%d] %s: vars=%v message=%q\n", i, rule.Type, rule.Vars, msg)
		}
	}

	return b.String()
}

// FormatYAML はhintをYAML形式で出力します。
func FormatYAML(hint *v1.TazunaHint) (string, error) {
	out, err := yaml.Marshal(hint)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal hint to YAML")
	}
	return string(out), nil
}

// FormatJSON はhintをJSON形式で出力します。
func FormatJSON(hint *v1.TazunaHint) (string, error) {
	out, err := json.MarshalIndent(hint, "", "  ")
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal hint to JSON")
	}
	return string(out), nil
}
