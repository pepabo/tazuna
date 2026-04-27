package hint_test

import (
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/hint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadHintFile(t *testing.T) {
	t.Parallel()
	t.Run("valid hint file", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/valid")
		require.NoError(t, err)
		require.NotNil(t, h)
		assert.Equal(t, "tazuna.pepabo.com/v1", h.APIVersion)
		assert.Equal(t, "TazunaHint", h.Kind)
		assert.Len(t, h.Vars, 5)
		assert.True(t, h.Vars["clusterName"].Required)
		assert.Equal(t, v1.HintVarTypeString, h.Vars["clusterName"].Type)
	})

	t.Run("valid hint file with rules", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/valid-with-rules")
		require.NoError(t, err)
		require.NotNil(t, h)
		assert.Len(t, h.Rules, 1)
		assert.Equal(t, v1.HintRuleTypeOneofRequired, h.Rules[0].Type)
		assert.Equal(t, v1.HintFormatHostname, h.Vars["hostname"].Format)
		assert.Equal(t, []string{"secretKey"}, h.Vars["apiKey"].RequiredWith)
		assert.Equal(t, []string{"serviceEndpoint"}, h.Vars["fallbackEndpoint"].RequiredWithout)
	})

	t.Run("file not found returns nil nil", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/empty")
		assert.NoError(t, err)
		assert.Nil(t, h)
	})

	t.Run("nonexistent directory returns nil nil", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/nonexistent")
		assert.NoError(t, err)
		assert.Nil(t, h)
	})
}

func TestValidateHint(t *testing.T) {
	t.Parallel()
	t.Run("valid hint", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/valid")
		require.NoError(t, err)
		err = hint.ValidateHint(h)
		assert.NoError(t, err)
	})

	t.Run("valid hint with all new fields", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/valid-with-rules")
		require.NoError(t, err)
		err = hint.ValidateHint(h)
		assert.NoError(t, err)
	})

	t.Run("nil hint is valid", func(t *testing.T) {
		err := hint.ValidateHint(nil)
		assert.NoError(t, err)
	})

	t.Run("invalid type", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/invalid-type")
		require.NoError(t, err)
		err = hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid type")
	})

	t.Run("required with default", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/required-with-default")
		require.NoError(t, err)
		err = hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "contradictory")
	})

	t.Run("format on non-string", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/format-on-non-string")
		require.NoError(t, err)
		err = hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "format")
		assert.Contains(t, err.Error(), "only valid for string")
	})

	t.Run("unknown format", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"foo": {Type: v1.HintVarTypeString, Format: "unknown_format"},
			},
		}
		err := hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown format")
	})

	t.Run("required_with referencing non-existent var", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"foo": {Type: v1.HintVarTypeString, RequiredWith: []string{"nonexistent"}},
			},
		}
		err := hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required_with referencing non-existent var")
	})

	t.Run("required_without referencing non-existent var", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"foo": {Type: v1.HintVarTypeString, RequiredWithout: []string{"nonexistent"}},
			},
		}
		err := hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required_without referencing non-existent var")
	})

	t.Run("required:true with required_with", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/required-with-and-required")
		require.NoError(t, err)
		err = hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "contradictory")
	})

	t.Run("required:true with required_without", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"foo": {Type: v1.HintVarTypeString, Required: true, RequiredWithout: []string{"bar"}},
				"bar": {Type: v1.HintVarTypeString},
			},
		}
		err := hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "contradictory")
	})

	t.Run("unknown rule type", func(t *testing.T) {
		h, err := hint.LoadHintFile("testdata/invalid-rule-type")
		require.NoError(t, err)
		err = hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown type")
	})

	t.Run("rule with less than 2 vars", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"foo": {Type: v1.HintVarTypeString},
			},
			Rules: []v1.HintRule{
				{Type: v1.HintRuleTypeOneofRequired, Vars: []string{"foo"}},
			},
		}
		err := hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least 2 vars")
	})

	t.Run("rule referencing non-existent var", func(t *testing.T) {
		h := &v1.TazunaHint{
			Vars: map[string]v1.HintVar{
				"foo": {Type: v1.HintVarTypeString},
			},
			Rules: []v1.HintRule{
				{Type: v1.HintRuleTypeOneofRequired, Vars: []string{"foo", "nonexistent"}},
			},
		}
		err := hint.ValidateHint(h)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "non-existent var")
	})
}

func TestValidateVarsAgainstHint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		hint    *v1.TazunaHint
		vars    map[string]v1.HelmFileVar
		wantErr bool
		errMsg  string
	}{
		{
			name: "string var with static is ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: true},
				},
			},
			vars: map[string]v1.HelmFileVar{
				"foo": {From: v1.HelmFileVarFromStatic, Static: strPtr("bar")},
			},
		},
		{
			name: "string var with env is ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: true},
				},
			},
			vars: map[string]v1.HelmFileVar{
				"foo": {From: v1.HelmFileVarFromEnv, Env: strPtr("MY_ENV")},
			},
		},
		{
			name: "string var with staticSlice is error",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: true},
				},
			},
			vars: map[string]v1.HelmFileVar{
				"foo": {From: v1.HelmFileVarFromStatic, StaticSlice: []string{"a"}},
			},
			wantErr: true,
			errMsg:  "expects type string",
		},
		{
			name: "string var with staticMap is error",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: true},
				},
			},
			vars: map[string]v1.HelmFileVar{
				"foo": {From: v1.HelmFileVarFromStatic, StaticMap: map[string]string{"k": "v"}},
			},
			wantErr: true,
			errMsg:  "expects type string",
		},
		{
			name: "slice var with staticSlice is ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeSlice, Required: true},
				},
			},
			vars: map[string]v1.HelmFileVar{
				"foo": {From: v1.HelmFileVarFromStatic, StaticSlice: []string{"a", "b"}},
			},
		},
		{
			name: "slice var with static string is error",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeSlice, Required: true},
				},
			},
			vars: map[string]v1.HelmFileVar{
				"foo": {From: v1.HelmFileVarFromStatic, Static: strPtr("bar")},
			},
			wantErr: true,
			errMsg:  "expects type slice",
		},
		{
			name: "map var with staticMap is ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeMap, Required: true},
				},
			},
			vars: map[string]v1.HelmFileVar{
				"foo": {From: v1.HelmFileVarFromStatic, StaticMap: map[string]string{"k": "v"}},
			},
		},
		{
			name: "map var with static string is error",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeMap, Required: true},
				},
			},
			vars: map[string]v1.HelmFileVar{
				"foo": {From: v1.HelmFileVarFromStatic, Static: strPtr("bar")},
			},
			wantErr: true,
			errMsg:  "expects type map",
		},
		{
			name: "unknown var passes through",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: true},
				},
			},
			vars: map[string]v1.HelmFileVar{
				"unknown": {From: v1.HelmFileVarFromStatic, Static: strPtr("bar")},
			},
		},
		{
			name: "nil hint is ok",
			hint: nil,
			vars: map[string]v1.HelmFileVar{
				"foo": {From: v1.HelmFileVarFromStatic, Static: strPtr("bar")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := hint.ValidateVarsAgainstHint(tt.hint, tt.vars)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMergeVarsWithHint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		hint    *v1.TazunaHint
		vars    map[string]any
		want    map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name: "required var provided",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: true},
				},
			},
			vars: map[string]any{"foo": "bar"},
			want: map[string]any{"foo": "bar"},
		},
		{
			name: "required var missing",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: true},
				},
			},
			vars:    map[string]any{},
			wantErr: true,
			errMsg:  "required but not provided",
		},
		{
			name: "optional with default, var provided",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: false, Default: "default-val"},
				},
			},
			vars: map[string]any{"foo": "override"},
			want: map[string]any{"foo": "override"},
		},
		{
			name: "optional with default, var not provided",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: false, Default: "default-val"},
				},
			},
			vars: map[string]any{},
			want: map[string]any{"foo": "default-val"},
		},
		{
			name: "optional without default, var provided",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: false},
				},
			},
			vars: map[string]any{"foo": "val"},
			want: map[string]any{"foo": "val"},
		},
		{
			name: "optional string without default, var not provided - zero value",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeString, Required: false},
				},
			},
			vars: map[string]any{},
			want: map[string]any{"foo": ""},
		},
		{
			name: "optional slice without default, var not provided - zero value",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeSlice, Required: false},
				},
			},
			vars: map[string]any{},
			want: map[string]any{"foo": []any{}},
		},
		{
			name: "optional map without default, var not provided - zero value",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"foo": {Type: v1.HintVarTypeMap, Required: false},
				},
			},
			vars: map[string]any{},
			want: map[string]any{"foo": map[string]any{}},
		},
		{
			name: "nil hint returns vars as-is",
			hint: nil,
			vars: map[string]any{"foo": "bar"},
			want: map[string]any{"foo": "bar"},
		},
		{
			name: "extra vars not in hint pass through",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"defined": {Type: v1.HintVarTypeString, Required: false},
				},
			},
			vars: map[string]any{"extra": "value"},
			want: map[string]any{"extra": "value", "defined": ""},
		},
		// required_with tests
		{
			name: "required_with: ref provided, self not provided -> error",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"apiKey":    {Type: v1.HintVarTypeString, RequiredWith: []string{"secretKey"}},
					"secretKey": {Type: v1.HintVarTypeString},
				},
			},
			vars:    map[string]any{"secretKey": "secret"},
			wantErr: true,
			errMsg:  "required because",
		},
		{
			name: "required_with: ref not provided, self not provided -> ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"apiKey":    {Type: v1.HintVarTypeString, RequiredWith: []string{"secretKey"}},
					"secretKey": {Type: v1.HintVarTypeString},
				},
			},
			vars: map[string]any{},
			want: map[string]any{"apiKey": "", "secretKey": ""},
		},
		{
			name: "required_with: ref provided, self provided -> ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"apiKey":    {Type: v1.HintVarTypeString, RequiredWith: []string{"secretKey"}},
					"secretKey": {Type: v1.HintVarTypeString},
				},
			},
			vars: map[string]any{"apiKey": "key", "secretKey": "secret"},
			want: map[string]any{"apiKey": "key", "secretKey": "secret"},
		},
		// required_without tests
		{
			name: "required_without: ref not provided, self not provided -> error",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"fallback": {Type: v1.HintVarTypeString, RequiredWithout: []string{"primary"}},
					"primary":  {Type: v1.HintVarTypeString},
				},
			},
			vars:    map[string]any{},
			wantErr: true,
			errMsg:  "required because none of",
		},
		{
			name: "required_without: ref provided, self not provided -> ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"fallback": {Type: v1.HintVarTypeString, RequiredWithout: []string{"primary"}},
					"primary":  {Type: v1.HintVarTypeString},
				},
			},
			vars: map[string]any{"primary": "value"},
			want: map[string]any{"primary": "value", "fallback": ""},
		},
		{
			name: "required_without: ref not provided, self provided -> ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"fallback": {Type: v1.HintVarTypeString, RequiredWithout: []string{"primary"}},
					"primary":  {Type: v1.HintVarTypeString},
				},
			},
			vars: map[string]any{"fallback": "fb-value"},
			want: map[string]any{"fallback": "fb-value", "primary": ""},
		},
		// oneof_required tests
		{
			name: "oneof_required: none provided -> error",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"a": {Type: v1.HintVarTypeString},
					"b": {Type: v1.HintVarTypeString},
				},
				Rules: []v1.HintRule{
					{Type: v1.HintRuleTypeOneofRequired, Vars: []string{"a", "b"}, Message: "a or b is required"},
				},
			},
			vars:    map[string]any{},
			wantErr: true,
			errMsg:  "a or b is required",
		},
		{
			name: "oneof_required: one provided -> ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"a": {Type: v1.HintVarTypeString},
					"b": {Type: v1.HintVarTypeString},
				},
				Rules: []v1.HintRule{
					{Type: v1.HintRuleTypeOneofRequired, Vars: []string{"a", "b"}},
				},
			},
			vars: map[string]any{"a": "val"},
			want: map[string]any{"a": "val", "b": ""},
		},
		{
			name: "oneof_required: multiple provided -> ok",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"a": {Type: v1.HintVarTypeString},
					"b": {Type: v1.HintVarTypeString},
				},
				Rules: []v1.HintRule{
					{Type: v1.HintRuleTypeOneofRequired, Vars: []string{"a", "b"}},
				},
			},
			vars: map[string]any{"a": "val-a", "b": "val-b"},
			want: map[string]any{"a": "val-a", "b": "val-b"},
		},
		// format tests
		{
			name: "format hostname: valid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"host": {Type: v1.HintVarTypeString, Format: v1.HintFormatHostname},
				},
			},
			vars: map[string]any{"host": "example.com"},
			want: map[string]any{"host": "example.com"},
		},
		{
			name: "format hostname: invalid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"host": {Type: v1.HintVarTypeString, Format: v1.HintFormatHostname},
				},
			},
			vars:    map[string]any{"host": "not a hostname!"},
			wantErr: true,
			errMsg:  "does not match format",
		},
		{
			name: "format url: valid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"endpoint": {Type: v1.HintVarTypeString, Format: v1.HintFormatURL},
				},
			},
			vars: map[string]any{"endpoint": "https://example.com/path"},
			want: map[string]any{"endpoint": "https://example.com/path"},
		},
		{
			name: "format url: invalid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"endpoint": {Type: v1.HintVarTypeString, Format: v1.HintFormatURL},
				},
			},
			vars:    map[string]any{"endpoint": "not-a-url"},
			wantErr: true,
			errMsg:  "does not match format",
		},
		{
			name: "format email: valid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"email": {Type: v1.HintVarTypeString, Format: v1.HintFormatEmail},
				},
			},
			vars: map[string]any{"email": "user@example.com"},
			want: map[string]any{"email": "user@example.com"},
		},
		{
			name: "format email: invalid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"email": {Type: v1.HintVarTypeString, Format: v1.HintFormatEmail},
				},
			},
			vars:    map[string]any{"email": "not-email"},
			wantErr: true,
			errMsg:  "does not match format",
		},
		{
			name: "format ip: valid ipv4",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"ip": {Type: v1.HintVarTypeString, Format: v1.HintFormatIP},
				},
			},
			vars: map[string]any{"ip": "192.168.1.1"},
			want: map[string]any{"ip": "192.168.1.1"},
		},
		{
			name: "format ip: valid ipv6",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"ip": {Type: v1.HintVarTypeString, Format: v1.HintFormatIP},
				},
			},
			vars: map[string]any{"ip": "::1"},
			want: map[string]any{"ip": "::1"},
		},
		{
			name: "format ip: invalid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"ip": {Type: v1.HintVarTypeString, Format: v1.HintFormatIP},
				},
			},
			vars:    map[string]any{"ip": "999.999.999.999"},
			wantErr: true,
			errMsg:  "does not match format",
		},
		{
			name: "format cidr: valid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"cidr": {Type: v1.HintVarTypeString, Format: v1.HintFormatCIDR},
				},
			},
			vars: map[string]any{"cidr": "10.0.0.0/8"},
			want: map[string]any{"cidr": "10.0.0.0/8"},
		},
		{
			name: "format cidr: invalid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"cidr": {Type: v1.HintVarTypeString, Format: v1.HintFormatCIDR},
				},
			},
			vars:    map[string]any{"cidr": "not-a-cidr"},
			wantErr: true,
			errMsg:  "does not match format",
		},
		{
			name: "format uuid: valid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"id": {Type: v1.HintVarTypeString, Format: v1.HintFormatUUID},
				},
			},
			vars: map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000"},
			want: map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000"},
		},
		{
			name: "format uuid: invalid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"id": {Type: v1.HintVarTypeString, Format: v1.HintFormatUUID},
				},
			},
			vars:    map[string]any{"id": "not-a-uuid"},
			wantErr: true,
			errMsg:  "does not match format",
		},
		{
			name: "format semver: valid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"version": {Type: v1.HintVarTypeString, Format: v1.HintFormatSemver},
				},
			},
			vars: map[string]any{"version": "1.2.3"},
			want: map[string]any{"version": "1.2.3"},
		},
		{
			name: "format semver: valid with v prefix",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"version": {Type: v1.HintVarTypeString, Format: v1.HintFormatSemver},
				},
			},
			vars: map[string]any{"version": "v1.2.3-alpha.1"},
			want: map[string]any{"version": "v1.2.3-alpha.1"},
		},
		{
			name: "format semver: invalid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"version": {Type: v1.HintVarTypeString, Format: v1.HintFormatSemver},
				},
			},
			vars:    map[string]any{"version": "not.semver"},
			wantErr: true,
			errMsg:  "does not match format",
		},
		{
			name: "format datetime: valid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"ts": {Type: v1.HintVarTypeString, Format: v1.HintFormatDatetime},
				},
			},
			vars: map[string]any{"ts": "2024-01-15T10:30:00Z"},
			want: map[string]any{"ts": "2024-01-15T10:30:00Z"},
		},
		{
			name: "format datetime: invalid",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"ts": {Type: v1.HintVarTypeString, Format: v1.HintFormatDatetime},
				},
			},
			vars:    map[string]any{"ts": "2024-01-15"},
			wantErr: true,
			errMsg:  "does not match format",
		},
		{
			name: "format: empty string (zero value injection) -> skip",
			hint: &v1.TazunaHint{
				Vars: map[string]v1.HintVar{
					"host": {Type: v1.HintVarTypeString, Format: v1.HintFormatHostname},
				},
			},
			vars: map[string]any{},
			want: map[string]any{"host": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := hint.MergeVarsWithHint(tt.hint, tt.vars)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
