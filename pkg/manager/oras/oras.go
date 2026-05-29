// Package oras - oras.go
//
// ORAS は OCI registry から artifact を pull し、tar.gz layer を展開した
// ローカルディレクトリを helmfile / kustomize manager に委譲する Manager 実装。
//
// 詳細は docs/adr/004-oras-manager.md および oras-manager-roadmap.md (commit 5) を参照。
package oras

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	v1 "github.com/pepabo/tazuna/api/v1"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// orasTracerName is the OpenTelemetry tracer name for the ORAS manager.
// pkg/manager/oras は循環依存を避けるため pkg/manager と別パッケージなので、
// ヘルパも別実装にする。span 名は親パッケージと合わせるため tazuna/manager を
// 使う。
const orasTracerName = "tazuna/manager"

// orasManifestSpanAttrs is the ORAS variant of pkg/manager.manifestSpanAttrs.
// ORAS マニフェスト固有の reference / digest 等は呼び出し側で追加する。
func orasManifestSpanAttrs(m v1.Manifest) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("manifest.name", m.Name),
		attribute.String("manifest.type", string(m.Type)),
		attribute.String("manifest.path", m.Path),
	}
	if m.ORAS != nil {
		attrs = append(attrs,
			attribute.String("oras.reference", m.ORAS.Reference),
			attribute.String("oras.delegate", string(m.ORAS.Delegate.Type)),
		)
	}
	return attrs
}

// orasRecordSpanError は err != nil なら span に error を記録する。
// pkg/manager.recordSpanError と同一処理だが、循環依存を避けるため別実装。
func orasRecordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// DelegateManager は ORAS が委譲する先のサブマネージャが満たすべき interface。
// pkg/manager.Manager と同一シグネチャのため、commit 6 で *manager.Helmfile /
// *manager.Kustomize をそのまま注入できる。
//
// 親パッケージ pkg/manager から pkg/manager/oras を import する想定 (commit 6) のため、
// 循環依存を避ける目的でローカルに interface を再定義している。
type DelegateManager interface {
	Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) error
	Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) error
	Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (string, error)
}

// ORAS は OCI registry からの pull と委譲先 manager の呼び出しを束ねる Manager 実装。
type ORAS struct {
	puller    Puller
	helmfile  DelegateManager
	kustomize DelegateManager
	pullOpts  PullOptions
}

// New は デフォルトの PullOptions で ORAS を生成します。
func New(puller Puller, helmfile, kustomize DelegateManager) *ORAS {
	return &ORAS{
		puller:    puller,
		helmfile:  helmfile,
		kustomize: kustomize,
	}
}

// NewWithOptions は明示的な PullOptions を伴う ORAS を生成します。
// CLI フラグ (--no-cache / --offline / --cache-dir) からの値を渡すために使います。
func NewWithOptions(puller Puller, helmfile, kustomize DelegateManager, opts PullOptions) *ORAS {
	o := New(puller, helmfile, kustomize)
	o.pullOpts = opts
	return o
}

// Apply は artifact を pull し、委譲先 manager の Apply を呼びます。
func (o *ORAS) Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) (retErr error) {
	ctx, span := otel.Tracer(orasTracerName).Start(ctx, "ORAS.Apply",
		trace.WithAttributes(orasManifestSpanAttrs(m)...))
	defer func() {
		orasRecordSpanError(span, retErr)
		span.End()
	}()

	delegate, delegated, err := o.prepareDelegate(ctx, logger, m)
	if err != nil {
		return err
	}
	return delegate.Apply(ctx, logger, delegated)
}

// Destroy は artifact を pull し、委譲先 manager の Destroy を呼びます。
func (o *ORAS) Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) (retErr error) {
	ctx, span := otel.Tracer(orasTracerName).Start(ctx, "ORAS.Destroy",
		trace.WithAttributes(orasManifestSpanAttrs(m)...))
	defer func() {
		orasRecordSpanError(span, retErr)
		span.End()
	}()

	delegate, delegated, err := o.prepareDelegate(ctx, logger, m)
	if err != nil {
		return err
	}
	return delegate.Destroy(ctx, logger, delegated)
}

// Build は artifact を pull し、委譲先 manager の Build を呼びます。
func (o *ORAS) Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (result string, retErr error) {
	ctx, span := otel.Tracer(orasTracerName).Start(ctx, "ORAS.Build",
		trace.WithAttributes(orasManifestSpanAttrs(m)...))
	defer func() {
		orasRecordSpanError(span, retErr)
		span.End()
	}()

	delegate, delegated, err := o.prepareDelegate(ctx, logger, m)
	if err != nil {
		return "", err
	}
	return delegate.Build(ctx, logger, delegated)
}

// prepareDelegate は pull → delegate 構築 → delegate 選択までの共通処理を行います。
func (o *ORAS) prepareDelegate(ctx context.Context, logger *slog.Logger, m v1.Manifest) (DelegateManager, v1.Manifest, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if m.ORAS == nil {
		return nil, v1.Manifest{}, fmt.Errorf("oras: manifest %q has no oras spec", m.Name)
	}

	result, err := o.puller.Pull(ctx, logger, *m.ORAS, o.pullOpts)
	if err != nil {
		return nil, v1.Manifest{}, fmt.Errorf("oras: pull %q: %w", m.ORAS.Reference, err)
	}
	logger.DebugContext(ctx, "oras pull resolved",
		slog.String("reference", m.ORAS.Reference),
		slog.String("digest", result.Digest),
		slog.String("local_path", result.LocalPath))

	delegated, err := buildDelegateManifest(m, result.LocalPath)
	if err != nil {
		return nil, v1.Manifest{}, err
	}

	delegate, err := o.selectDelegate(m.ORAS.Delegate.Type)
	if err != nil {
		return nil, v1.Manifest{}, err
	}
	return delegate, delegated, nil
}

// selectDelegate は delegate type に対応する DelegateManager を返します。
func (o *ORAS) selectDelegate(t v1.ORASDelegateType) (DelegateManager, error) {
	switch t {
	case v1.ORASDelegateTypeHelmfile:
		if o.helmfile == nil {
			return nil, fmt.Errorf("oras: helmfile delegate is not configured")
		}
		return o.helmfile, nil
	case v1.ORASDelegateTypeKustomize:
		if o.kustomize == nil {
			return nil, fmt.Errorf("oras: kustomize delegate is not configured")
		}
		return o.kustomize, nil
	default:
		return nil, fmt.Errorf("oras: unsupported delegate type %q", t)
	}
}

// buildDelegateManifest は pull 結果のローカルパスを基に、
// 委譲先 manager に渡す v1.Manifest を構築します。
// spec.Target が指定されていれば LocalPath/target をサブパスとして解決し、
// LocalPath を脱出する target は拒否します。
func buildDelegateManifest(m v1.Manifest, localPath string) (v1.Manifest, error) {
	spec := m.ORAS
	target := strings.TrimPrefix(spec.Target, "/")
	fullPath := localPath
	if target != "" {
		fullPath = filepath.Join(localPath, target)
	}

	// path traversal 防止: target が localPath を脱出しないことを確認する。
	absLocal, err := filepath.Abs(localPath)
	if err != nil {
		return v1.Manifest{}, fmt.Errorf("oras: resolve local path: %w", err)
	}
	absFull, err := filepath.Abs(fullPath)
	if err != nil {
		return v1.Manifest{}, fmt.Errorf("oras: resolve target path: %w", err)
	}
	rel, err := filepath.Rel(absLocal, absFull)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return v1.Manifest{}, fmt.Errorf("oras: invalid target path %q (escapes artifact root)", spec.Target)
	}

	delegated := v1.Manifest{
		Name:        m.Name,
		Description: m.Description,
		Tags:        m.Tags,
		Path:        fullPath,
		Tests:       m.Tests,
	}
	switch spec.Delegate.Type {
	case v1.ORASDelegateTypeHelmfile:
		delegated.Type = v1.ManifestTypeHelmfile
		delegated.Helmfile = spec.Delegate.Helmfile
	case v1.ORASDelegateTypeKustomize:
		delegated.Type = v1.ManifestTypeKustomize
		delegated.Kustomize = spec.Delegate.Kustomize
	default:
		return v1.Manifest{}, fmt.Errorf("oras: unsupported delegate type %q", spec.Delegate.Type)
	}
	return delegated, nil
}
