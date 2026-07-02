package genesissecret

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

// EnvFileProvider は .env 形式のファイルから値を取得する SecretProvider 実装です。
// 1 行ごとに `KEY=VALUE` の形式でパースし、`s.Items` のキーを envfile 側の KEY と
// 読み替えて map を返します。
//
// 仕様上の制約:
//   - シェル展開や複数行値はサポートしない。
//   - 行頭の '#' から始まる行と空行は無視する。
//   - 値が一対のシングル/ダブルクォートで囲まれている場合のみ剥がす。
//   - preferLabel は envfile に label の概念がないため無視される。
//   - s.URI は使用しない (envfile では item 概念がない)。
//   - ファイルは Fetch のたびに毎回読み直す (キャッシュしない)。
type EnvFileProvider struct {
	path string
}

// NewEnvFileProvider creates a new EnvFileProvider that reads from the given path.
func NewEnvFileProvider(path string) *EnvFileProvider {
	return &EnvFileProvider{path: path}
}

// Fetch implements SecretProvider.
func (p *EnvFileProvider) Fetch(_ context.Context, s v1.GenesisSecretGenerate) (map[string]string, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read envfile %q", p.path)
	}

	fetched, err := parseEnvFile(data)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse envfile %q", p.path)
	}

	return mapTo(fetched, s.Items)
}

var _ SecretProvider = &EnvFileProvider{}

// parseEnvFile は dotenv 形式のバイト列をパースして key/value マップを返します。
// 不正な行 (KEY が無い、'=' が無い) はエラーにする。
func parseEnvFile(data []byte) (map[string]string, error) {
	out := map[string]string{}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		// コメント行と空行は無視する
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		idx := strings.Index(line, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("invalid envfile line %d: %q", lineNo, raw)
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		if key == "" {
			return nil, fmt.Errorf("invalid envfile line %d: empty key", lineNo)
		}

		out[key] = stripQuotes(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.WithStack(err)
	}

	return out, nil
}

// stripQuotes は値が一対の同じ種類のクォートで囲まれている場合のみ剥がす。
func stripQuotes(v string) string {
	if len(v) < 2 {
		return v
	}
	first := v[0]
	last := v[len(v)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}
