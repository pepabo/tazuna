package tfstate_test

import (
	"os"
	"testing"

	"github.com/pepabo/tazuna/pkg/tfstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadExample(t *testing.T) *tfstate.State {
	t.Helper()
	data, err := os.ReadFile("testdata/example.tfstate")
	require.NoError(t, err)
	st, err := tfstate.Parse(data)
	require.NoError(t, err)
	return st
}

func TestParse_Metadata(t *testing.T) {
	t.Parallel()
	st := loadExample(t)
	assert.Equal(t, 4, st.Version)
	assert.Equal(t, "1.9.8", st.TerraformVersion)
	assert.Equal(t, int64(42), st.Serial)
	assert.Len(t, st.Resources, 5)
	assert.True(t, st.Outputs["db_password"].Sensitive)
}

func TestParse_Errors(t *testing.T) {
	t.Parallel()

	v3, err := os.ReadFile("testdata/v3.tfstate")
	require.NoError(t, err)
	_, err = tfstate.Parse(v3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported tfstate format version 3")

	_, err = tfstate.Parse([]byte("not json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode tfstate")

	_, err = tfstate.Parse([]byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version 0")
}

func TestState_Lookup(t *testing.T) {
	t.Parallel()
	st := loadExample(t)

	tests := []struct {
		addr string
		want any
	}{
		{"output.vpc_id", "vpc-0123456789abcdef0"},
		{"output.account_id", int64(123456789012)},
		{"output.ratio", 0.25},
		{"output.enabled", true},
		{"output.private_subnet_ids", []any{"subnet-a", "subnet-b"}},
		{"output.private_subnet_ids[1]", "subnet-b"},
		{"output.private_subnet_ids.0", "subnet-a"},
		{"output.cluster.name", "example"},
		{`output.cluster["endpoint"]`, "https://example.invalid"},
		{`output.cluster.tags["env"]`, "test"},
		{"output.cluster.tags", map[string]any{"env": "test"}},
		{"output.db_password", "s3cr3t"},
		{"output.nothing", nil},
		{"aws_vpc.main.id", "vpc-0123456789abcdef0"},
		{"aws_vpc.main.tags.Name", "main"},
		{"aws_subnet.private[1].id", "subnet-b"},
		{"data.aws_caller_identity.current.account_id", "123456789012"},
		{`module.app["web"].aws_iam_role.this["primary"].arn`, "arn:aws:iam::123456789012:role/web"},
		{"module.network.module.subnets[0].aws_route_table.rt.id", "rtb-1"},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()
			got, err := st.LookupString(tt.addr)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestState_Lookup_Errors(t *testing.T) {
	t.Parallel()
	st := loadExample(t)

	tests := []struct {
		addr    string
		wantErr []string
	}{
		{"output.missing", []string{`output "missing" not found`, "account_id", "vpc_id"}},
		{"output.cluster.missing", []string{`key "missing" not found`, "endpoint, name, tags"}},
		{"output.private_subnet_ids[5]", []string{"index 5 out of range (length 2)"}},
		{`output.private_subnet_ids["a"]`, []string{"requires an integer index"}},
		{"output.cluster[0]", []string{"cannot use integer index on an object"}},
		{"output.vpc_id.foo", []string{"cannot traverse into a string value"}},
		{"output.nothing.foo", []string{"cannot traverse into null"}},
		{"aws_vpc.other.id", []string{"resource aws_vpc.other not found", "aws_subnet.private", "data.aws_caller_identity.current"}},
		{"aws_subnet.private.id", []string{"uses count or for_each; specify an index", "[0], [1]"}},
		{"aws_subnet.private[9].id", []string{"resource instance aws_subnet.private[9] not found"}},
		{"aws_vpc.main[0].id", []string{"resource instance aws_vpc.main[0] not found", "(no index)"}},
		{"aws_vpc.main.nope", []string{`key "nope" not found`}},
		{"aws_iam_role.this.arn", []string{"resource aws_iam_role.this not found"}},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()
			_, err := st.LookupString(tt.addr)
			require.Error(t, err)
			for _, w := range tt.wantErr {
				assert.Contains(t, err.Error(), w)
			}
			// エラーメッセージに値 (sensitive を含む) を出していないこと
			assert.NotContains(t, err.Error(), "s3cr3t")
		})
	}
}

func TestFormatValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   any
		want string
	}{
		{"string", "abc", "abc"},
		{"int", int64(123456789012), "123456789012"},
		{"float", 0.25, "0.25"},
		{"large float", 1e21, "1000000000000000000000"},
		{"bool", false, "false"},
		{"list", []any{"a", int64(1)}, `["a",1]`},
		{"map", map[string]any{"b": "2", "a": "1"}, `{"a":"1","b":"2"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tfstate.FormatValue(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := tfstate.FormatValue(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null")
}
