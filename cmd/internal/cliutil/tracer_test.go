package cliutil_test

import (
	"context"
	"testing"

	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestSetupTracer_EmptyEndpoint は --otlp-endpoint 未指定 (= 空文字) のときに
// noop TracerProvider が返り、shutdown が安全に呼べることを確認します。
// 実 OTLP 接続テストは外部依存になるため対象外。
func TestSetupTracer_EmptyEndpoint(t *testing.T) {
	t.Parallel()

	tp, shutdown, err := cliutil.SetupTracer(context.Background(), "", true)
	if err != nil {
		t.Fatalf("SetupTracer returned error: %v", err)
	}
	if tp == nil {
		t.Fatal("SetupTracer returned nil TracerProvider")
	}
	// noop provider と同じ実装が返ることを検証する。SDK TracerProvider が返って
	// しまうと CLI のデフォルト動作で OTLP 接続を試みてしまうため重要。
	if _, ok := tp.(noop.TracerProvider); !ok {
		t.Errorf("SetupTracer with empty endpoint returned %T, want noop.TracerProvider", tp)
	}
	if shutdown == nil {
		t.Fatal("SetupTracer returned nil shutdown func")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("noop shutdown returned error: %v", err)
	}
}

// TestSetupTracer_NoopTracerUsable は返された TracerProvider から Tracer/Span を
// 取得しても panic しないこと、Span 操作が no-op として完結することを確認します。
func TestSetupTracer_NoopTracerUsable(t *testing.T) {
	t.Parallel()

	tp, shutdown, err := cliutil.SetupTracer(context.Background(), "", false)
	if err != nil {
		t.Fatalf("SetupTracer returned error: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Errorf("shutdown returned error: %v", err)
		}
	}()

	tracer := tp.Tracer("tazuna-test")
	_, span := tracer.Start(context.Background(), "test-span")
	span.End()
}
