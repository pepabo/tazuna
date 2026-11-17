package runner_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/tfstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func loadTFStateFixture(t *testing.T) (v1.Tazuna, string) {
	t.Helper()
	path := filepath.Join("testdata", "tfstate", "tazuna.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var tz v1.Tazuna
	require.NoError(t, yaml.Unmarshal(data, &tz))
	return tz, path
}

// TestBuild_TFState は include ファイルの template 関数・helmfile vars (from: tfstate)・
// tfstate Secret provider の 3 経路すべてで local tfstate の値が解決されることを確認します。
func TestBuild_TFState(t *testing.T) {
	t.Parallel()
	tz, path := loadTFStateFixture(t)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := runner.NewTazunaRunner(logger, nil, nil)

	out, err := r.Build(context.Background(), tz, path)
	require.NoError(t, err)

	// helmfile: release namespace は include の {{ tfstate }}、値は from: tfstate
	assert.Contains(t, out, "namespace: platform")
	assert.Contains(t, out, `vpc-id: "vpc-0123456789abcdef0"`)
	assert.Contains(t, out, `subnets: "subnet-a,subnet-b"`)

	// genesissecret: tfstate provider 経由の値が Secret の data に入る
	// (build は stringData で出力する)
	assert.Contains(t, out, "PASSWORD: s3cr3t")
	assert.Contains(t, out, "HOST: db.example.invalid")
	assert.Contains(t, out, `PORT: "5432"`)

	// state は 1 回だけ読まれ、ログに値は出ない
	assert.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("tfstate: loaded state")))
	assert.NotContains(t, logs.String(), "s3cr3t")
}

// TestBuild_TFStateSharedResolver は cmd 層から渡された Resolver が使われることを確認します。
func TestBuild_TFStateSharedResolver(t *testing.T) {
	t.Parallel()
	tz, path := loadTFStateFixture(t)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	res, err := tfstate.NewResolver(logger, tz.Spec.TFStates, filepath.Dir(path), nil)
	require.NoError(t, err)
	// 事前に state を読み込ませておく (tazuna.yaml 本体の描画に相当)
	_, err = res.Lookup(context.Background(), "infra", "output.namespace")
	require.NoError(t, err)

	r := runner.NewTazunaRunner(nil, nil, nil, runner.WithTFStateResolver(res))
	_, err = r.Build(context.Background(), tz, path)
	require.NoError(t, err)
	assert.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("tfstate: loaded state")))
}

func TestBuild_TFStateProviderUnknownSource(t *testing.T) {
	t.Parallel()
	tz, path := loadTFStateFixture(t)
	tz.Spec.Providers[0].TFState.Source = "nope"

	_, err := runner.NewTazunaRunner(nil, nil, nil).Build(context.Background(), tz, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tfstate source "nope" is not declared in spec.tfstates`)
}
