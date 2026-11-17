package genesissecret_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLookuper struct {
	values  map[string]any
	sources []string
}

func (f *fakeLookuper) Lookup(_ context.Context, source, address string) (any, error) {
	f.sources = append(f.sources, source)
	v, ok := f.values[address]
	if !ok {
		return nil, errors.Errorf("%q not found", address)
	}
	return v, nil
}

func TestTFStateProvider_Fetch(t *testing.T) {
	t.Parallel()
	l := &fakeLookuper{values: map[string]any{
		"output.db_password":            "s3cr3t",
		"output.db_port":                int64(5432),
		"output.enabled":                true,
		"output.hosts":                  []any{"a", "b"},
		"aws_db_instance.main.endpoint": "db.example.invalid:5432",
		"output.nothing":                nil,
	}}
	p := genesissecret.NewTFStateProvider(l, "database")

	got, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		URI: "ignored",
		Items: map[string]v1.GenesisSecretGenerateItem{
			"output.db_password":            {MapTo: "PASSWORD"},
			"output.db_port":                {MapTo: "PORT"},
			"output.enabled":                {MapTo: "ENABLED"},
			"output.hosts":                  {MapTo: "HOSTS"},
			"aws_db_instance.main.endpoint": {MapTo: "ENDPOINT"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"PASSWORD": "s3cr3t",
		"PORT":     "5432",
		"ENABLED":  "true",
		"HOSTS":    `["a","b"]`,
		"ENDPOINT": "db.example.invalid:5432",
	}, got)
	for _, s := range l.sources {
		assert.Equal(t, "database", s)
	}

	_, err = p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{"output.nothing": {MapTo: "X"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"output.nothing" cannot be used as a secret value`)

	_, err = p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{"output.missing": {MapTo: "X"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"output.missing" not found`)
}

func TestTFStateProvider_NilLookuper(t *testing.T) {
	t.Parallel()
	p := genesissecret.NewTFStateProvider(nil, "database")
	_, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{"output.x": {MapTo: "X"}},
	})
	require.ErrorContains(t, err, "has no resolver")
}
