package tfstate

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sort"
	"sync"
	"text/template"
	"time"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "tazuna/tfstate"

// Resolver は spec.tfstates[] で宣言された名前付き tfstate ソースを解決します。
//
// state は各ソースが初めて参照されたときに 1 回だけ backend から読み込まれ、
// 以降は同じ Resolver 内でキャッシュされます。読み込みはソース単位で排他される
// ため、dependsOn による並列 apply から同時に参照されても安全です。
// 読み込みに失敗した場合もその結果 (エラー) がキャッシュされ、同じ実行中に
// 何度も backend へアクセスし直すことはありません。
//
// ログには「どのソースをどこから読んだか」だけを出し、state 内の値は一切出力しません。
type Resolver struct {
	logger  *slog.Logger
	sources map[string]*source
}

type source struct {
	name        string
	backendType v1.TFStateBackendType
	backend     Backend

	once  sync.Once
	state *State
	err   error
}

// NewResolver は spec.tfstates[] から Resolver を構築します。backend の生成
// (設定値の検査) のみを行い、state の読み込みは Lookup まで遅延します。
// factories が nil の場合は DefaultFactories() を使います。logger が nil の場合はログを破棄します。
func NewResolver(logger *slog.Logger, sources []v1.TFStateSource, baseDir string, factories Factories) (*Resolver, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if factories == nil {
		factories = DefaultFactories()
	}
	r := &Resolver{
		logger:  logger,
		sources: make(map[string]*source, len(sources)),
	}
	for i, s := range sources {
		if s.Name == "" {
			return nil, errors.Errorf("tfstates[%d]: name is required", i)
		}
		if _, dup := r.sources[s.Name]; dup {
			return nil, errors.Errorf("tfstates[%d]: duplicate name %q", i, s.Name)
		}
		b, err := factories.New(s.Backend, baseDir)
		if err != nil {
			return nil, errors.Wrapf(err, "tfstates[%d] (%q)", i, s.Name)
		}
		r.sources[s.Name] = &source{name: s.Name, backendType: s.Backend.Type, backend: b}
	}
	return r, nil
}

// Has は name のソースが宣言されているかを返します。nil の Resolver では常に false です。
func (r *Resolver) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.sources[name]
	return ok
}

// Lookup は source の tfstate から address の値を返します。
// 戻り値は string / bool / int64 / float64 / []any / map[string]any / nil のいずれかです。
func (r *Resolver) Lookup(ctx context.Context, sourceName, address string) (any, error) {
	if r == nil {
		return nil, errors.Errorf("tfstate source %q is not declared (spec.tfstates is empty)", sourceName)
	}
	addr, err := ParseAddress(address)
	if err != nil {
		return nil, errors.Wrapf(err, "tfstate source %q", sourceName)
	}
	st, err := r.State(ctx, sourceName)
	if err != nil {
		return nil, err
	}
	v, err := st.Lookup(addr)
	if err != nil {
		return nil, errors.Wrapf(err, "tfstate source %q: failed to look up %q", sourceName, address)
	}
	r.logger.DebugContext(ctx, "tfstate: resolved value",
		slog.String("source", sourceName),
		slog.String("address", address),
		slog.String("value_kind", valueKind(v)))
	return v, nil
}

// LookupString は Lookup した値を FormatValue で文字列化して返します。
func (r *Resolver) LookupString(ctx context.Context, sourceName, address string) (string, error) {
	v, err := r.Lookup(ctx, sourceName, address)
	if err != nil {
		return "", err
	}
	s, err := FormatValue(v)
	if err != nil {
		return "", errors.Wrapf(err, "tfstate source %q: %q", sourceName, address)
	}
	return s, nil
}

// State は source の tfstate を (必要なら backend から読み込んで) 返します。
func (r *Resolver) State(ctx context.Context, sourceName string) (*State, error) {
	if r == nil {
		return nil, errors.Errorf("tfstate source %q is not declared (spec.tfstates is empty)", sourceName)
	}
	s, ok := r.sources[sourceName]
	if !ok {
		return nil, errors.Errorf("tfstate source %q is not declared in spec.tfstates (declared: %s)", sourceName, candidates(r.names()))
	}
	s.once.Do(func() {
		s.state, s.err = r.load(ctx, s)
	})
	return s.state, s.err
}

func (r *Resolver) load(ctx context.Context, s *source) (st *State, retErr error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "TFState.Load",
		trace.WithAttributes(
			attribute.String("tfstate.source", s.name),
			attribute.String("tfstate.backend", string(s.backendType)),
		))
	defer func() {
		if retErr != nil {
			span.RecordError(retErr)
			span.SetStatus(codes.Error, retErr.Error())
		}
		span.End()
	}()

	attrs := append([]slog.Attr{
		slog.String("source", s.name),
		slog.String("backend", string(s.backendType)),
	}, s.backend.LogAttrs()...)
	r.logger.LogAttrs(ctx, slog.LevelDebug, "tfstate: loading state", attrs...)

	start := time.Now()
	data, err := s.backend.Read(ctx)
	if err != nil {
		r.logger.LogAttrs(ctx, slog.LevelDebug, "tfstate: failed to read state",
			append(attrs, slog.String("error", err.Error()))...)
		return nil, errors.Wrapf(err, "tfstate source %q (%s backend): failed to read state", s.name, s.backendType)
	}
	st, err = Parse(data)
	if err != nil {
		return nil, errors.Wrapf(err, "tfstate source %q (%s backend): failed to parse state", s.name, s.backendType)
	}
	span.SetAttributes(
		attribute.Int64("tfstate.serial", st.Serial),
		attribute.String("tfstate.terraform_version", st.TerraformVersion),
	)

	r.logger.LogAttrs(ctx, slog.LevelDebug, "tfstate: loaded state",
		append(attrs,
			slog.Int("bytes", len(data)),
			slog.Int64("serial", st.Serial),
			slog.String("terraform_version", st.TerraformVersion),
			slog.Int("outputs", len(st.Outputs)),
			slog.Int("resources", len(st.Resources)),
			slog.Duration("elapsed", time.Since(start)),
		)...)
	return st, nil
}

func (r *Resolver) names() []string {
	out := make([]string, 0, len(r.sources))
	for n := range r.sources {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func valueKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "list"
	default:
		return kindOf(v)
	}
}

// Template 関数名。pkg/tmpl の stub と同じ名前でなければなりません。
const (
	TemplateFuncName     = "tfstate"
	TemplateFuncNameJSON = "tfstateJSON"
)

// TemplateFuncs は tazuna.yaml / include ファイルの描画で使う template 関数を返します。
//
//   - {{ tfstate "<source>" "<address>" }}: 値を FormatValue で文字列化したもの
//   - {{ tfstateJSON "<source>" "<address>" }}: 値の JSON 表現 (YAML の flow 値として埋め込める)
func (r *Resolver) TemplateFuncs(ctx context.Context) template.FuncMap {
	return template.FuncMap{
		TemplateFuncName: func(sourceName, address string) (string, error) {
			return r.LookupString(ctx, sourceName, address)
		},
		TemplateFuncNameJSON: func(sourceName, address string) (string, error) {
			v, err := r.Lookup(ctx, sourceName, address)
			if err != nil {
				return "", err
			}
			b, err := json.Marshal(v)
			if err != nil {
				return "", errors.Wrapf(err, "tfstate source %q: failed to encode %q as JSON", sourceName, address)
			}
			return string(b), nil
		},
	}
}

func sortedStrings(s []string) []string {
	sort.Strings(s)
	return s
}

// Lookuper は tfstate から値を引く操作だけを切り出した interface です。
// manager / Secret provider はこの interface に依存することで、テストで fake を
// 差し込めます。*Resolver が実装します (nil の *Resolver でも未宣言エラーを返して安全に動作します)。
type Lookuper interface {
	Lookup(ctx context.Context, sourceName, address string) (any, error)
}

var _ Lookuper = (*Resolver)(nil)
