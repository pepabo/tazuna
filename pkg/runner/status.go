package runner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/resource"
	"github.com/pepabo/tazuna/pkg/state"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceStatus は status コマンドが扱う managed リソースの判定結果。
type ResourceStatus string

const (
	// ResourceStatusReady は対象リソースがクラスタ上に存在し、Ready 判定された状態を表す。
	ResourceStatusReady ResourceStatus = "Ready"
	// ResourceStatusNotReady は対象リソースがクラスタ上に存在するが、Ready 判定されなかった状態を表す。
	ResourceStatusNotReady ResourceStatus = "NotReady"
	// ResourceStatusMissing は state には記録されているが、クラスタ上に存在しない状態を表す。
	ResourceStatusMissing ResourceStatus = "Missing"
	// ResourceStatusError は live 取得時に NotFound 以外のエラーが発生したことを表す。
	ResourceStatusError ResourceStatus = "Error"
)

// Status はステートConfigMapに記録された managed リソースを走査し、
// 各リソースの Ready / NotReady / Missing を一覧表示する。
func (t *TazunaRunner) Status(
	ctx context.Context,
	tazuna v1.Tazuna,
	tazunaYAMLPath string,
	w io.Writer,
) (retErr error) {
	ctx, span := otel.Tracer(runnerTracerName).Start(ctx, "tazuna.Status",
		trace.WithAttributes(
			attribute.String("tazuna.yaml.path", tazunaYAMLPath),
			attribute.Int("manifests.count", len(tazuna.Spec.Manifests)),
		))
	defer func() {
		recordRunnerSpanErr(span, retErr)
		span.End()
	}()

	if err := t.expandIncludes(ctx, &tazuna, tazunaYAMLPath); err != nil {
		return errors.WithStack(err)
	}

	baseDir := filepath.Dir(tazunaYAMLPath)
	t.ConvertManifestPathFromCwd(baseDir, &tazuna)

	store := state.NewConfigMapStateStore(t.k8sClient)

	for i, m := range tazuna.Spec.Manifests {
		if m.Name == "" {
			t.logger.WarnContext(ctx, "manifest has no name, skipping status",
				slog.Int("index", i),
				slog.String("type", string(m.Type)))
			continue
		}

		data, err := store.Get(ctx, m.Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get state for manifest %q", m.Name)
		}

		if len(data.Entries) == 0 {
			if _, err := fmt.Fprintf(w, "Manifest: %s\n  (no state)\n\n", m.Name); err != nil {
				return errors.WithStack(err)
			}
			continue
		}

		rows, err := t.collectStatusRows(ctx, data)
		if err != nil {
			return errors.WithStack(err)
		}

		if _, err := fmt.Fprintf(w, "Manifest: %s\n", m.Name); err != nil {
			return errors.WithStack(err)
		}
		if err := writeStatusRows(w, rows); err != nil {
			return errors.WithStack(err)
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// statusRow は status コマンドの 1 行分の情報。
type statusRow struct {
	Status    ResourceStatus
	Kind      string
	Namespace string
	Name      string
}

// collectStatusRows は state の各 Entry についてライブクラスタから状況を取得し、
// 表示用の行データを構築する。ライブ GET は read-only なので errgroup で並列化する。
func (t *TazunaRunner) collectStatusRows(
	ctx context.Context,
	data *state.StateData,
) ([]statusRow, error) {
	// 安定した出力のために state key でソートする
	keys := make([]string, 0, len(data.Entries))
	for k := range data.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	results := make([]*statusRow, len(keys))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(liveGetConcurrency)
	for i, keyStr := range keys {
		g.Go(func() error {
			results[i] = t.collectStatusRow(gctx, keyStr)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	rows := make([]statusRow, 0, len(keys))
	for _, row := range results {
		if row != nil {
			rows = append(rows, *row)
		}
	}

	return rows, nil
}

// collectStatusRow は 1 つの state key についてライブクラスタから状況を取得する。
// state key がパースできない場合は警告を出して nil を返す。
func (t *TazunaRunner) collectStatusRow(ctx context.Context, keyStr string) *statusRow {
	parsed, err := state.ParseStateKey(keyStr)
	if err != nil {
		t.logger.WarnContext(ctx, "failed to parse state key, skipping",
			slog.String("key", keyStr),
			slog.String("error", err.Error()))
		return nil
	}

	obj, err := buildUnstructuredFromStateKey(keyStr)
	if err != nil {
		t.logger.WarnContext(ctx, "failed to build unstructured from state key, skipping",
			slog.String("key", keyStr),
			slog.String("error", err.Error()))
		return nil
	}

	row := &statusRow{
		Kind:      parsed.Kind,
		Namespace: parsed.Namespace,
		Name:      parsed.Name,
	}

	getErr := t.k8sClient.Get(ctx, client.ObjectKey{Namespace: parsed.Namespace, Name: parsed.Name}, obj)
	switch {
	case getErr == nil:
		ready, rerr := resource.IsReady(obj)
		if rerr != nil {
			t.logger.WarnContext(ctx, "failed to determine readiness, marking as Error",
				slog.String("key", keyStr),
				slog.String("error", rerr.Error()))
			row.Status = ResourceStatusError
		} else if ready {
			row.Status = ResourceStatusReady
		} else {
			row.Status = ResourceStatusNotReady
		}
	case apierrors.IsNotFound(getErr):
		row.Status = ResourceStatusMissing
	default:
		t.logger.WarnContext(ctx, "failed to get live resource, marking as Error",
			slog.String("key", keyStr),
			slog.String("error", getErr.Error()))
		row.Status = ResourceStatusError
	}

	return row
}

// writeStatusRows は statusRow 群を整形して w に書き出す。
func writeStatusRows(w io.Writer, rows []statusRow) error {
	if _, err := fmt.Fprintf(w, "  %-9s %-20s %s\n", "STATUS", "KIND", "NAMESPACE/NAME"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "  %-9s %-20s %s\n",
			row.Status,
			row.Kind,
			formatNamespacedName(row.Namespace, row.Name),
		); err != nil {
			return err
		}
	}
	return nil
}

// formatNamespacedName は cluster-scoped リソースの場合は name のみ、
// namespaced リソースの場合は "namespace/name" を返す。
func formatNamespacedName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return fmt.Sprintf("%s/%s", namespace, name)
}
