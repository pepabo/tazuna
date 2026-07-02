package cliutil

import (
	"context"
	"log/slog"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// tracerShutdownTimeout is the timeout used when flushing pending spans on
// shutdown. tazuna is a short-lived CLI so we keep this small enough to avoid
// hanging the user's terminal even if the OTLP endpoint is unreachable.
const tracerShutdownTimeout = 5 * time.Second

// SetupTracer initializes an OpenTelemetry TracerProvider for tazuna.
//
// When endpoint is empty, a no-op TracerProvider is returned and shutdown is a
// no-op. This keeps span-instrumented code paths usable in tests and in normal
// CLI runs without any external dependency.
//
// When endpoint is non-empty, an OTLP/gRPC exporter is wired into a SDK
// TracerProvider tagged with the "tazuna" service name. The returned shutdown
// function must be deferred at the top of each cobra RunE so that pending
// spans are flushed before the CLI exits.
//
// The returned TracerProvider is also installed as the process-global provider
// via otel.SetTracerProvider so that callers using otel.Tracer(...) pick it up
// automatically.
func SetupTracer(ctx context.Context, endpoint string, insecure bool) (trace.TracerProvider, func(context.Context) error, error) {
	if endpoint == "" {
		tp := noop.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return tp, func(context.Context) error { return nil }, nil
	}

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create OTLP trace exporter for %s", endpoint)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			attribute.String("service.name", "tazuna"),
		),
	)
	if err != nil {
		// resource.Merge は schema URL の差異がない限り失敗しないが、念のため
		// exporter を閉じてから上位にエラーを返す。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), tracerShutdownTimeout)
		defer cancel()
		_ = exporter.Shutdown(shutdownCtx)
		return nil, nil, errors.Wrap(err, "failed to build OTLP resource")
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	shutdown := func(shutdownCtx context.Context) error {
		// 呼び出し元が context.Background() を渡してきても CLI が長時間ぶら下がらない
		// よう、タイムアウト付きの context にラップする。
		ctx, cancel := context.WithTimeout(shutdownCtx, tracerShutdownTimeout)
		defer cancel()
		return tp.Shutdown(ctx)
	}
	return tp, shutdown, nil
}

// ShutdownTracerWithWarn は shutdown を実行し、失敗した場合は警告ログを出す。
// span flush の失敗でコマンド自体を失敗させたくない defer 用途に使う
// (完全に無音で握り潰すと flush 失敗に気づけないため警告は残す)。
func ShutdownTracerWithWarn(shutdown func(context.Context) error) {
	if err := shutdown(context.Background()); err != nil {
		slog.Warn("failed to flush traces on tracer shutdown", "error", err.Error())
	}
}

// SetupTracerFromCmd reads the persistent --otlp-endpoint and --otlp-insecure
// flags from cmd and calls SetupTracer. It is a thin convenience wrapper to
// keep RunE bodies free of flag plumbing.
func SetupTracerFromCmd(ctx context.Context, cmd *cobra.Command) (trace.TracerProvider, func(context.Context) error, error) {
	endpoint, err := cmd.Flags().GetString("otlp-endpoint")
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	insecure, err := cmd.Flags().GetBool("otlp-insecure")
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	return SetupTracer(ctx, endpoint, insecure)
}
