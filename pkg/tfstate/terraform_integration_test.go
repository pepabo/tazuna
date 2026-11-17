//go:build integration

package tfstate_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/tfstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// terraformConfig は provider のダウンロードが不要な組み込みの terraform_data のみで
// outputs / count / for_each / module / sensitive を一通り含む state を作ります。
const terraformConfig = `
terraform {
  required_version = ">= 1.4"
}

resource "terraform_data" "single" {
  input = { name = "single", port = 8080 }
}

resource "terraform_data" "counted" {
  count = 2
  input = "counted-${count.index}"
}

resource "terraform_data" "each" {
  for_each = toset(["web", "batch"])
  input    = "each-${each.key}"
}

module "child" {
  source = "./child"
  for_each = { primary = "p" }
  label    = each.value
}

output "vpc_id" {
  value = "vpc-integration"
}

output "subnet_ids" {
  value = ["subnet-1", "subnet-2"]
}

output "cluster" {
  value = { name = "it", nodes = 3, enabled = true }
}

output "db_password" {
  value     = "integration-secret"
  sensitive = true
}

output "workspace" {
  value = terraform.workspace
}
`

const childConfig = `
variable "label" { type = string }

resource "terraform_data" "this" {
  input = "child-${var.label}"
}
`

func runTerraform(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("terraform", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"TF_IN_AUTOMATION=1",
		"TF_INPUT=0",
		"CHECKPOINT_DISABLE=1",
		"TF_DATA_DIR="+filepath.Join(dir, ".terraform"),
	)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "terraform %v failed:\n%s", args, out)
}

// TestTerraformGeneratedState は実際の terraform が書き出した state (default と
// workspace の両方) を local backend 経由で読み、各種アドレスが解決できることを確認します。
func TestTerraformGeneratedState(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform is not installed; skipping")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(terraformConfig), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "child"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "child", "main.tf"), []byte(childConfig), 0o600))

	runTerraform(t, dir, "init", "-no-color")
	runTerraform(t, dir, "apply", "-auto-approve", "-no-color")
	runTerraform(t, dir, "workspace", "new", "-no-color", "staging")
	runTerraform(t, dir, "apply", "-auto-approve", "-no-color")

	r, err := tfstate.NewResolver(nil, []v1.TFStateSource{
		{Name: "default", Backend: v1.TFStateBackend{Type: "local", Local: &v1.TFStateLocalBackend{Path: "terraform.tfstate"}}},
		{Name: "staging", Backend: v1.TFStateBackend{Type: "local", Local: &v1.TFStateLocalBackend{Path: "terraform.tfstate.d/staging/terraform.tfstate"}}},
	}, dir, nil)
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		address string
		want    any
	}{
		{"output.vpc_id", "vpc-integration"},
		{"output.subnet_ids", []any{"subnet-1", "subnet-2"}},
		{"output.subnet_ids[1]", "subnet-2"},
		{"output.cluster.name", "it"},
		{"output.cluster.nodes", int64(3)},
		{"output.cluster.enabled", true},
		{"output.db_password", "integration-secret"},
		{"output.workspace", "default"},
		// terraform_data の input / output は dynamic 型の属性で、state 上は
		// {"value": ..., "type": ...} という形で保存される (terraform の仕様)。
		{"terraform_data.single.input.value.name", "single"},
		{"terraform_data.single.input.value.port", int64(8080)},
		{"terraform_data.counted[1].input.value", "counted-1"},
		{`terraform_data.each["batch"].output.value`, "each-batch"},
		{`module.child["primary"].terraform_data.this.output.value`, "child-p"},
	}
	for _, tt := range tests {
		got, err := r.Lookup(ctx, "default", tt.address)
		require.NoErrorf(t, err, "lookup %s", tt.address)
		assert.Equalf(t, tt.want, got, "lookup %s", tt.address)
	}

	got, err := r.Lookup(ctx, "staging", "output.workspace")
	require.NoError(t, err)
	assert.Equal(t, "staging", got)

	st, err := r.State(ctx, "default")
	require.NoError(t, err)
	assert.Equal(t, tfstate.SupportedStateVersion, st.Version)
	assert.NotEmpty(t, st.TerraformVersion)
	assert.True(t, st.Outputs["db_password"].Sensitive)
}
