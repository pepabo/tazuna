package tfstate_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"text/template"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/tfstate"
	"github.com/pepabo/tazuna/pkg/tmpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingBackend は Read の呼び出し回数を数える fake backend です。
type countingBackend struct {
	data  []byte
	err   error
	reads atomic.Int32
}

func (b *countingBackend) Read(context.Context) ([]byte, error) {
	b.reads.Add(1)
	return b.data, b.err
}

func (b *countingBackend) LogAttrs() []slog.Attr {
	return []slog.Attr{slog.String("location", "fake://example")}
}

const fakeType v1.TFStateBackendType = "fake"

// newFakeResolver は "fake" backend type だけを持つ Resolver を作ります。
// Factories を差し替えられることが、新しい backend を追加するときの拡張点でもあります。
func newFakeResolver(t *testing.T, logger *slog.Logger, backends map[string]*countingBackend) *tfstate.Resolver {
	t.Helper()
	var sources []v1.TFStateSource
	for name := range backends {
		// fake backend 側でどのソースか識別できるよう、local.path に名前を載せる
		sources = append(sources, v1.TFStateSource{Name: name, Backend: v1.TFStateBackend{
			Type: fakeType, Local: &v1.TFStateLocalBackend{Path: name},
		}})
	}
	factories := tfstate.Factories{
		fakeType: func(cfg v1.TFStateBackend, _ string) (tfstate.Backend, error) {
			b, ok := backends[cfg.Local.Path]
			if !ok {
				return nil, errors.Errorf("unexpected source %q", cfg.Local.Path)
			}
			return b, nil
		},
	}
	r, err := tfstate.NewResolver(logger, sources, "", factories)
	require.NoError(t, err)
	return r
}

func exampleBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/example.tfstate")
	require.NoError(t, err)
	return data
}

func TestResolver_LookupCachesState(t *testing.T) {
	t.Parallel()
	b := &countingBackend{data: exampleBytes(t)}
	r := newFakeResolver(t, nil, map[string]*countingBackend{"network": b})
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := r.Lookup(ctx, "network", "output.vpc_id")
			assert.NoError(t, err)
			assert.Equal(t, "vpc-0123456789abcdef0", v)
		}()
	}
	wg.Wait()

	v, err := r.Lookup(ctx, "network", "aws_subnet.private[0].id")
	require.NoError(t, err)
	assert.Equal(t, "subnet-a", v)

	assert.Equal(t, int32(1), b.reads.Load(), "state must be read from the backend only once")
}

func TestResolver_ReadErrorIsCached(t *testing.T) {
	t.Parallel()
	b := &countingBackend{err: errors.New("boom")}
	r := newFakeResolver(t, nil, map[string]*countingBackend{"network": b})

	for range 3 {
		_, err := r.Lookup(context.Background(), "network", "output.vpc_id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `tfstate source "network" (fake backend): failed to read state`)
		assert.Contains(t, err.Error(), "boom")
	}
	assert.Equal(t, int32(1), b.reads.Load())
}

func TestResolver_Errors(t *testing.T) {
	t.Parallel()
	r := newFakeResolver(t, nil, map[string]*countingBackend{
		"network": {data: exampleBytes(t)},
		"broken":  {data: []byte(`{"version":3}`)},
	})
	ctx := context.Background()

	_, err := r.Lookup(ctx, "unknown", "output.vpc_id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tfstate source "unknown" is not declared in spec.tfstates (declared: broken, network)`)

	_, err = r.Lookup(ctx, "network", "output.")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tfstate address")

	_, err = r.Lookup(ctx, "network", "output.missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tfstate source "network": failed to look up "output.missing"`)

	_, err = r.Lookup(ctx, "broken", "output.x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state")

	_, err = r.LookupString(ctx, "network", "output.nothing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value is null")
}

func TestResolver_Nil(t *testing.T) {
	t.Parallel()
	var r *tfstate.Resolver
	assert.False(t, r.Has("x"))
	_, err := r.Lookup(context.Background(), "x", "output.y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.tfstates is empty")
}

func TestNewResolver_Errors(t *testing.T) {
	t.Parallel()
	ok := v1.TFStateBackend{Type: "local", Local: &v1.TFStateLocalBackend{Path: "x"}}

	_, err := tfstate.NewResolver(nil, []v1.TFStateSource{{Backend: ok}}, "", nil)
	require.ErrorContains(t, err, "tfstates[0]: name is required")

	_, err = tfstate.NewResolver(nil, []v1.TFStateSource{{Name: "a", Backend: ok}, {Name: "a", Backend: ok}}, "", nil)
	require.ErrorContains(t, err, `tfstates[1]: duplicate name "a"`)

	_, err = tfstate.NewResolver(nil, []v1.TFStateSource{{Name: "a", Backend: v1.TFStateBackend{Type: "s3"}}}, "", nil)
	require.ErrorContains(t, err, `tfstates[0] ("a"): s3 backend: s3 config is required`)
}

func TestResolver_LogsDoNotContainValues(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := newFakeResolver(t, logger, map[string]*countingBackend{"network": {data: exampleBytes(t)}})

	v, err := r.LookupString(context.Background(), "network", "output.db_password")
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", v)

	logs := buf.String()
	assert.Contains(t, logs, "tfstate: loading state")
	assert.Contains(t, logs, "tfstate: loaded state")
	assert.Contains(t, logs, "source=network")
	assert.Contains(t, logs, "backend=fake")
	assert.Contains(t, logs, "location=fake://example")
	assert.Contains(t, logs, "serial=42")
	assert.Contains(t, logs, "terraform_version=1.9.8")
	assert.Contains(t, logs, "tfstate: resolved value")
	assert.Contains(t, logs, "address=output.db_password")
	assert.Contains(t, logs, "value_kind=string")
	assert.NotContains(t, logs, "s3cr3t")
}

func TestResolver_TemplateFuncs(t *testing.T) {
	t.Parallel()
	r := newFakeResolver(t, nil, map[string]*countingBackend{"network": {data: exampleBytes(t)}})

	src := strings.Join([]string{
		`vpc: {{ tfstate "network" "output.vpc_id" }}`,
		`account: {{ tfstate "network" "output.account_id" }}`,
		`subnets: {{ tfstateJSON "network" "output.private_subnet_ids" }}`,
		`subnetsStr: '{{ tfstate "network" "output.private_subnet_ids" }}'`,
		`quoted: {{ tfstateJSON "network" "output.vpc_id" }}`,
		`env: {{ .Environment }}`,
	}, "\n")
	out, err := tmpl.RenderWithFuncs("t.yaml", []byte(src), tmpl.Data{Environment: "prod"}, r.TemplateFuncs(context.Background()))
	require.NoError(t, err)
	assert.Equal(t, strings.Join([]string{
		`vpc: vpc-0123456789abcdef0`,
		`account: 123456789012`,
		`subnets: ["subnet-a","subnet-b"]`,
		`subnetsStr: '["subnet-a","subnet-b"]'`,
		`quoted: "vpc-0123456789abcdef0"`,
		`env: prod`,
	}, "\n"), string(out))

	_, err = tmpl.RenderWithFuncs("t.yaml", []byte(`{{ tfstate "network" "output.missing" }}`), tmpl.Data{}, r.TemplateFuncs(context.Background()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `output "missing" not found`)
}

// TestTemplateFuncNamesMatchStubs は tmpl の stub と Resolver の関数名がずれていないことを保証します。
func TestTemplateFuncNamesMatchStubs(t *testing.T) {
	t.Parallel()
	var r *tfstate.Resolver
	real := r.TemplateFuncs(context.Background())
	stub := tmpl.StubFuncs()
	assert.ElementsMatch(t, keys(real), keys(stub))
	assert.Contains(t, keys(real), tfstate.TemplateFuncName)
	assert.Contains(t, keys(real), tfstate.TemplateFuncNameJSON)
}

func keys(m template.FuncMap) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
