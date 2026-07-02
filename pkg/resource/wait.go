package resource

import (
	"context"
	"log/slog"
	"time"

	"github.com/cockroachdb/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// defaultWaitTimeoutSeconds は WaitForReady のデフォルトタイムアウト秒数。
const defaultWaitTimeoutSeconds = 300

// WaitForReady は objects がすべて Ready になるまでポーリングして待機します。
// timeoutSeconds が 0 以下の場合はデフォルト (300 秒) を使います。
// helmfile の wait: true など、apply / sync の双方から利用されます。
func WaitForReady(ctx context.Context, c client.Client, logger *slog.Logger, objects []client.Object, timeoutSeconds int) error {
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultWaitTimeoutSeconds
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	for _, obj := range objects {
		if err := waitForResource(timeoutCtx, c, logger, obj); err != nil {
			return errors.Wrapf(err, "failed to wait for resource %s/%s", obj.GetNamespace(), obj.GetName())
		}
	}

	return nil
}

// waitForResource は単一のリソースが Ready になるまで待機します。
func waitForResource(ctx context.Context, c client.Client, logger *slog.Logger, obj client.Object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	logger.InfoContext(ctx, "waiting for resource to be ready",
		slog.String("namespace", obj.GetNamespace()),
		slog.String("name", obj.GetName()),
		slog.String("kind", gvk.Kind))

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.Errorf("timeout waiting for %s %s/%s to be ready", gvk.Kind, obj.GetNamespace(), obj.GetName())
		case <-ticker.C:
			ready, err := isResourceReady(ctx, c, obj)
			if err != nil {
				return errors.WithStack(err)
			}
			if ready {
				logger.InfoContext(ctx, "resource is ready",
					slog.String("namespace", obj.GetNamespace()),
					slog.String("name", obj.GetName()),
					slog.String("kind", gvk.Kind))
				return nil
			}
		}
	}
}

// isResourceReady はリソースをライブ取得して Ready 状態かどうかを確認します。
func isResourceReady(ctx context.Context, c client.Client, obj client.Object) (bool, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	key := client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(gvk)
	if err := c.Get(ctx, key, current); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// リソースがまだ存在しない場合は ready ではない
			return false, nil
		}
		return false, errors.WithStack(err)
	}

	return IsReady(current)
}
