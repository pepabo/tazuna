package hint_test

import (
	"encoding/json"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/hint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFormatHuman(t *testing.T) {
	t.Parallel()
	t.Run("with vars", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"clusterName": {
					Type:        v1.HintVarTypeString,
					Required:    true,
					Description: "cluster name",
				},
				"environment": {
					Type:        v1.HintVarTypeString,
					Required:    false,
					Default:     "production",
					Description: "environment name",
				},
			},
		}
		out := hint.FormatHuman(h, "system/example")
		assert.Contains(t, out, "Vars for system/example:")
		assert.Contains(t, out, "clusterName (string, required)")
		assert.Contains(t, out, "environment (string, optional)")
		assert.Contains(t, out, "default: production")
	})

	t.Run("nil hint", func(t *testing.T) {
		out := hint.FormatHuman(nil, "system/example")
		assert.Contains(t, out, "No vars defined")
	})

	t.Run("empty vars", func(t *testing.T) {
		h := &v1.TazunaHint{Vars: map[string]v1.HintVar{}}
		out := hint.FormatHuman(h, "system/example")
		assert.Contains(t, out, "No vars defined")
	})

	t.Run("with format", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"hostname": {
					Type:   v1.HintVarTypeString,
					Format: v1.HintFormatHostname,
				},
			},
		}
		out := hint.FormatHuman(h, "test")
		assert.Contains(t, out, "format:hostname")
	})

	t.Run("with required_with", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"apiKey": {
					Type:         v1.HintVarTypeString,
					RequiredWith: []string{"secretKey"},
				},
				"secretKey": {
					Type: v1.HintVarTypeString,
				},
			},
		}
		out := hint.FormatHuman(h, "test")
		assert.Contains(t, out, "required_with: [secretKey]")
	})

	t.Run("with required_without", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"fallback": {
					Type:            v1.HintVarTypeString,
					RequiredWithout: []string{"primary"},
				},
				"primary": {
					Type: v1.HintVarTypeString,
				},
			},
		}
		out := hint.FormatHuman(h, "test")
		assert.Contains(t, out, "required_without: [primary]")
	})

	t.Run("with rules", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"a": {Type: v1.HintVarTypeString},
				"b": {Type: v1.HintVarTypeString},
			},
			Rules: []v1.HintRule{
				{Type: v1.HintRuleTypeOneofRequired, Vars: []string{"a", "b"}, Message: "a or b is required"},
			},
		}
		out := hint.FormatHuman(h, "test")
		assert.Contains(t, out, "Rules:")
		assert.Contains(t, out, "oneof_required")
		assert.Contains(t, out, "a or b is required")
	})

	t.Run("with rules no message", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"a": {Type: v1.HintVarTypeString},
				"b": {Type: v1.HintVarTypeString},
			},
			Rules: []v1.HintRule{
				{Type: v1.HintRuleTypeOneofRequired, Vars: []string{"a", "b"}},
			},
		}
		out := hint.FormatHuman(h, "test")
		assert.Contains(t, out, "(no message)")
	})
}

func TestFormatYAML(t *testing.T) {
	t.Parallel()
	h := &v1.TazunaHint{
		APIVersion: "tazuna.pepabo.com/v1",
		Kind:       "TazunaHint",
		Vars: map[string]v1.HintVar{
			"foo": {Type: v1.HintVarTypeString, Required: true},
		},
	}
	out, err := hint.FormatYAML(h)
	require.NoError(t, err)

	var parsed v1.TazunaHint
	err = yaml.Unmarshal([]byte(out), &parsed)
	require.NoError(t, err)
	assert.Equal(t, h.APIVersion, parsed.APIVersion)
	assert.Equal(t, h.Vars["foo"].Type, parsed.Vars["foo"].Type)
}

func TestFormatJSON(t *testing.T) {
	t.Parallel()
	h := &v1.TazunaHint{
		APIVersion: "tazuna.pepabo.com/v1",
		Kind:       "TazunaHint",
		Vars: map[string]v1.HintVar{
			"foo": {Type: v1.HintVarTypeString, Required: true},
		},
	}
	out, err := hint.FormatJSON(h)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal([]byte(out), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "tazuna.pepabo.com/v1", parsed["APIVersion"])
}
