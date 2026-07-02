package manager

import (
	"context"
	"log/slog"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/resource"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// managerTracerName is the OpenTelemetry tracer name used by all built-in
// manager implementations. Using a single name keeps spans grouped under one
// instrumentation library in the collector / backend UI.
const managerTracerName = "tazuna/manager"

// manifestSpanAttrs builds the standard attribute.KeyValue slice attached to
// every manager-level span so that manifest identity (name/type/path) is
// visible in the trace UI without having to drill into individual events.
func manifestSpanAttrs(m v1.Manifest) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("manifest.name", m.Name),
		attribute.String("manifest.type", string(m.Type)),
		attribute.String("manifest.path", m.Path),
	}
}

// recordSpanError marks span as failed with err and records it. err == nil は
// no-op で、defer の最後で呼んでも安全に動作する。
func recordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

type Kustomize struct {
	client client.Client
}

func NewKustomize(client client.Client) *Kustomize {
	return &Kustomize{client}
}

// renderKustomizeYAML は kustomize build を実行して YAML を返す。
// Apply / Destroy / Build で共通利用するレンダリング部分。
func renderKustomizeYAML(path string) ([]byte, error) {
	fs := filesys.MakeFsOnDisk()
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	resourceMap, err := kustomizer.Run(fs, path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out, err := resourceMap.AsYaml()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return out, nil
}

// renderObjects は kustomize build の結果を client.Object 群に変換する。
// m.Kustomize の nil デフォルト補完もここで行う。
func (k *Kustomize) renderObjects(m *v1.Manifest) ([]client.Object, error) {
	out, err := renderKustomizeYAML(m.Path)
	if err != nil {
		return nil, err
	}

	if m.Kustomize == nil {
		m.Kustomize = &v1.ManifestKustomize{}
	}

	return manifest.ConvertManifestsToObjects(out, m.Kustomize.DefaultNamespace)
}

// Destroy implements Manager.
func (k *Kustomize) Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) (retErr error) {
	ctx, span := otel.Tracer(managerTracerName).Start(ctx, "Kustomize.Destroy",
		trace.WithAttributes(manifestSpanAttrs(m)...))
	defer func() {
		recordSpanError(span, retErr)
		span.End()
	}()

	objects, err := k.renderObjects(&m)
	if err != nil {
		return errors.WithStack(err)
	}
	logger.DebugContext(ctx, "successfully converted manifests to objects", slog.Int("count", len(objects)))

	for _, obj := range objects {
		logger.DebugContext(ctx, "trying to delete an object", slog.String("namespace", obj.GetNamespace()), slog.String("name", obj.GetName()), slog.String("kind", obj.GetObjectKind().GroupVersionKind().Kind))
		if err := resource.DeleteObject(ctx, k.client, obj); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// Apply implements Manager.
func (k *Kustomize) Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) (objects []client.Object, retErr error) {
	ctx, span := otel.Tracer(managerTracerName).Start(ctx, "Kustomize.Apply",
		trace.WithAttributes(manifestSpanAttrs(m)...))
	defer func() {
		recordSpanError(span, retErr)
		span.End()
	}()

	objects, err := k.renderObjects(&m)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	logger.DebugContext(ctx, "successfully converted manifests to objects", slog.Int("count", len(objects)))
	span.SetAttributes(attribute.Int("manifest.objects", len(objects)))

	for _, obj := range objects {
		logger.DebugContext(ctx, "trying to create or update an object", slog.String("namespace", obj.GetNamespace()), slog.String("name", obj.GetName()))
		if err := resource.CreateOrUpdateForObject(ctx, k.client, obj); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return objects, nil
}

var _ Manager = &Kustomize{}

// Build implements Manager.
func (k *Kustomize) Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (out string, retErr error) {
	_, span := otel.Tracer(managerTracerName).Start(ctx, "Kustomize.Build",
		trace.WithAttributes(manifestSpanAttrs(m)...))
	defer func() {
		recordSpanError(span, retErr)
		span.End()
	}()

	raw, err := renderKustomizeYAML(m.Path)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return string(raw), nil
}
