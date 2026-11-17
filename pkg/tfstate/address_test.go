package tfstate_test

import (
	"testing"

	"github.com/pepabo/tazuna/pkg/tfstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want tfstate.Address
	}{
		{
			in:   "output.vpc_id",
			want: tfstate.Address{Output: "vpc_id"},
		},
		{
			in: `output.cluster.tags["env"]`,
			want: tfstate.Address{Output: "cluster", Path: []tfstate.Step{
				{Name: "tags"}, {Index: "env"},
			}},
		},
		{
			in:   "output.private_subnet_ids[1]",
			want: tfstate.Address{Output: "private_subnet_ids", Path: []tfstate.Step{{Index: int64(1)}}},
		},
		{
			in: "aws_vpc.main.id",
			want: tfstate.Address{Mode: tfstate.ResourceModeManaged, Type: "aws_vpc", Name: "main",
				Path: []tfstate.Step{{Name: "id"}}},
		},
		{
			in: "aws_subnet.private[0].id",
			want: tfstate.Address{Mode: tfstate.ResourceModeManaged, Type: "aws_subnet", Name: "private",
				Index: int64(0), Path: []tfstate.Step{{Name: "id"}}},
		},
		{
			in: "data.aws_caller_identity.current.account_id",
			want: tfstate.Address{Mode: tfstate.ResourceModeData, Type: "aws_caller_identity", Name: "current",
				Path: []tfstate.Step{{Name: "account_id"}}},
		},
		{
			in: `module.app["web"].aws_iam_role.this["primary"].arn`,
			want: tfstate.Address{Module: `module.app["web"]`, Mode: tfstate.ResourceModeManaged,
				Type: "aws_iam_role", Name: "this", Index: "primary", Path: []tfstate.Step{{Name: "arn"}}},
		},
		{
			in: "module.network.module.subnets[0].aws_route_table.rt.id",
			want: tfstate.Address{Module: "module.network.module.subnets[0]", Mode: tfstate.ResourceModeManaged,
				Type: "aws_route_table", Name: "rt", Path: []tfstate.Step{{Name: "id"}}},
		},
		{
			in:   `output.weird["a\"b"]`,
			want: tfstate.Address{Output: "weird", Path: []tfstate.Step{{Index: `a"b`}}},
		},
		{
			// output 名や attribute 名に "output" / "module" / "data" 自体が出てきてもよい
			in: "data.data.module.id",
			want: tfstate.Address{Mode: tfstate.ResourceModeData, Type: "data", Name: "module",
				Path: []tfstate.Step{{Name: "id"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := tfstate.ParseAddress(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want.Output, got.Output)
			assert.Equal(t, tt.want.Module, got.Module)
			assert.Equal(t, tt.want.Mode, got.Mode)
			assert.Equal(t, tt.want.Type, got.Type)
			assert.Equal(t, tt.want.Name, got.Name)
			assert.Equal(t, tt.want.Index, got.Index)
			if len(tt.want.Path) == 0 {
				assert.Empty(t, got.Path)
			} else {
				assert.Equal(t, tt.want.Path, got.Path)
			}
			assert.Equal(t, tt.in, got.String())
		})
	}
}

func TestParseAddress_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		wantErr string
	}{
		{"", "address is empty"},
		{"output", "output name is missing"},
		{"output.", "must not end with '.'"},
		{".output.x", "unexpected '.'"},
		{"output..x", "unexpected '.'"},
		{"output.x[", "unterminated index"},
		{"output.x[-1]", "non-negative integer"},
		{"output.x[abc]", "non-negative integer"},
		{`output.x["abc]`, "unterminated string index"},
		{"output.x]", "unexpected character"},
		{"aws_vpc", "resource name is missing"},
		{"aws_vpc.main", "attribute of aws_vpc.main is missing"},
		{"aws_subnet.private[0]", "attribute of aws_subnet.private[0] is missing"},
		{"aws_subnet.private[0][1].id", "unexpected index"},
		{"module.x", "resource type is missing"},
		{"module", "module name is missing"},
		{"[0].id", "unexpected '['"},
		{"output.a b", "unexpected character"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			_, err := tfstate.ParseAddress(tt.in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
